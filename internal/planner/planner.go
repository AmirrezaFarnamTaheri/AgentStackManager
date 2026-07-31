package planner

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/model"
)

type Request struct {
	Profile           string            `json:"profile"`
	Include           []string          `json:"include,omitempty"`
	Exclude           []string          `json:"exclude,omitempty"`
	AllowCredentialed bool              `json:"allowCredentialed,omitempty"`
	AllowUpgrades     bool              `json:"allowUpgrades,omitempty"`
	ProviderOverrides map[string]string `json:"providerOverrides,omitempty"`
}

const DefaultPlanTTL = 10 * time.Minute

func Build(c model.Catalog, inventory model.Inventory, request Request) (model.Plan, error) {
	if request.Profile == "" {
		request.Profile = "core"
	}
	profile, ok := c.ProfileByID(request.Profile)
	if !ok {
		return model.Plan{}, fmt.Errorf("unknown profile %q", request.Profile)
	}

	selected := make(map[string]bool)
	excluded := make(map[string]bool)
	included := make(map[string]bool)
	for _, id := range profile.Components {
		selected[id] = true
	}
	for _, id := range request.Include {
		if _, exists := c.ComponentByID(id); !exists {
			return model.Plan{}, fmt.Errorf("unknown included component %q", id)
		}
		included[id] = true
		selected[id] = true
	}
	for _, id := range request.Exclude {
		if _, exists := c.ComponentByID(id); !exists {
			return model.Plan{}, fmt.Errorf("unknown excluded component %q", id)
		}
		if included[id] {
			return model.Plan{}, fmt.Errorf("component %q is both included and excluded", id)
		}
		excluded[id] = true
		delete(selected, id)
	}

	activeProviders := make(map[string]string)
	for _, component := range c.Components {
		if component.Preferred && component.Capability != "" {
			activeProviders[component.Capability] = component.ID
		}
	}
	for capability, componentID := range request.ProviderOverrides {
		component, exists := c.ComponentByID(componentID)
		if !exists {
			return model.Plan{}, fmt.Errorf("provider override for %q references unknown component %q", capability, componentID)
		}
		if component.Capability != capability {
			return model.Plan{}, fmt.Errorf("component %q does not provide capability %q", componentID, capability)
		}
		if excluded[componentID] {
			return model.Plan{}, fmt.Errorf("provider override for %q references excluded component %q", capability, componentID)
		}
		activeProviders[capability] = componentID
		selected[componentID] = true
	}
	if err := expandDependencies(c, selected, excluded); err != nil {
		return model.Plan{}, err
	}
	if err := reconcileActiveProviders(c, selected, activeProviders); err != nil {
		return model.Plan{}, err
	}

	actions := make([]model.PlanAction, 0, len(selected))
	for _, component := range c.Components {
		if !selected[component.ID] {
			continue
		}
		item := inventory.Items[component.ID]
		action := model.PlanAction{
			ComponentID: component.ID,
			Name:        component.Name,
			Install:     component.Install,
			Selected:    true,
		}

		if component.CredentialRequired && !request.AllowCredentialed {
			action.Kind = model.ActionConsentRequired
			action.Reason = "credential or login integration requires explicit consent"
			actions = append(actions, action)
			continue
		}

		if component.Capability != "" {
			if active, exists := activeProviders[component.Capability]; exists && active != component.ID {
				if item.Installed {
					action.Kind = model.ActionPreserveInactive
					action.Reason = fmt.Sprintf("preserved on disk; %s is the active provider for %s", active, component.Capability)
				} else {
					action.Kind = model.ActionSkipDominated
					action.Reason = fmt.Sprintf("not installed because %s is the active provider for %s", active, component.Capability)
				}
				actions = append(actions, action)
				continue
			}
		}

		if item.Incompatible && !request.AllowUpgrades {
			action.Kind = model.ActionConsentRequired
			action.Reason = "existing version is incompatible; explicit upgrade consent is required"
		} else if item.Incompatible {
			action.Kind = model.ActionRepair
			action.Upgrade = true
			action.Reason = "existing version is incompatible and will be upgraded under explicit consent"
		} else if item.Broken {
			action.Kind = model.ActionRepair
			action.Reason = "AgentStack-managed component is no longer detectable and can be restored"
		} else if item.Installed {
			action.Kind = model.ActionKeep
			action.Reason = "existing installation detected and preserved"
		} else if component.Install.Kind == model.InstallRouter {
			action.Kind = model.ActionConfigure
			action.Reason = "add to the AgentStack-managed router profile; child server starts lazily"
		} else if component.Install.Kind == model.InstallNone || component.Install.Kind == model.InstallManual {
			action.Kind = model.ActionSkip
			action.Reason = "manual component; no automatic mutation"
			if component.Install.LoginHint != "" {
				action.Reason += "; next step: " + component.Install.LoginHint
			}
		} else {
			action.Kind = model.ActionInstall
			action.Reason = "missing selected component"
		}
		actions = append(actions, action)
	}

	sort.SliceStable(actions, func(i, j int) bool {
		left, _ := c.ComponentByID(actions[i].ComponentID)
		right, _ := c.ComponentByID(actions[j].ComponentID)
		if left.Tier == right.Tier {
			return left.Name < right.Name
		}
		return tierRank(left.Tier) < tierRank(right.Tier)
	})
	ordered, err := orderByDependencies(c, actions)
	if err != nil {
		return model.Plan{}, err
	}

	return model.Plan{
		Profile:     request.Profile,
		GeneratedAt: time.Now().UTC(),
		Actions:     ordered,
		Providers:   activeProviders,
	}, nil
}

// Seal binds a reviewed plan to the exact catalog and machine inventory that
// produced it. The digest excludes only its own digest field.
func Seal(c model.Catalog, inventory model.Inventory, plan model.Plan, ttl time.Duration) (model.Plan, error) {
	if ttl <= 0 {
		ttl = DefaultPlanTTL
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return model.Plan{}, fmt.Errorf("generate plan id: %w", err)
	}
	plan.ID = "plan-" + hex.EncodeToString(idBytes)
	plan.ExpiresAt = time.Now().UTC().Add(ttl)
	var err error
	plan.CatalogHash, err = integrity.DigestJSON(c)
	if err != nil {
		return model.Plan{}, err
	}
	plan.InventoryHash, err = InventoryDigest(inventory)
	if err != nil {
		return model.Plan{}, err
	}
	plan.Digest, err = PlanDigest(plan)
	if err != nil {
		return model.Plan{}, err
	}
	return plan, nil
}

func PlanDigest(plan model.Plan) (string, error) {
	plan.Digest = ""
	return integrity.DigestJSON(plan)
}

func InventoryDigest(inventory model.Inventory) (string, error) {
	inventory.GeneratedAt = time.Time{}
	inventory.Revision = ""
	inventory.RawSources = nil
	return integrity.DigestJSON(inventory)
}

func reconcileActiveProviders(c model.Catalog, selected map[string]bool, active map[string]string) error {
	selectedByCapability := map[string][]string{}
	for _, component := range c.Components {
		if selected[component.ID] && component.Capability != "" {
			selectedByCapability[component.Capability] = append(selectedByCapability[component.Capability], component.ID)
		}
	}
	for capability, candidates := range selectedByCapability {
		if selected[active[capability]] {
			continue
		}
		sort.Strings(candidates)
		switch len(candidates) {
		case 0:
			delete(active, capability)
		case 1:
			active[capability] = candidates[0]
		default:
			return fmt.Errorf("multiple selected providers for capability %q (%v); use --provider %s=component-id", capability, candidates, capability)
		}
	}
	for capability, componentID := range active {
		if !selected[componentID] && len(selectedByCapability[capability]) == 0 {
			delete(active, capability)
		}
	}
	return nil
}

func expandDependencies(c model.Catalog, selected, excluded map[string]bool) error {
	queue := make([]string, 0, len(selected))
	for id := range selected {
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		component, ok := c.ComponentByID(id)
		if !ok {
			return fmt.Errorf("selected component %q is missing from catalog", id)
		}
		for _, dependency := range component.DependsOn {
			if excluded[dependency] {
				return fmt.Errorf("component %q depends on excluded component %q", component.ID, dependency)
			}
			if !selected[dependency] {
				selected[dependency] = true
				queue = append(queue, dependency)
			}
		}
	}
	return nil
}

func orderByDependencies(c model.Catalog, actions []model.PlanAction) ([]model.PlanAction, error) {
	byID := make(map[string]model.PlanAction, len(actions))
	for _, action := range actions {
		byID[action.ComponentID] = action
	}
	const (
		unseen = iota
		visiting
		visited
	)
	state := map[string]int{}
	ordered := make([]model.PlanAction, 0, len(actions))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("dependency cycle while planning component %q", id)
		case visited:
			return nil
		}
		state[id] = visiting
		component, ok := c.ComponentByID(id)
		if !ok {
			return fmt.Errorf("plan references unknown component %q", id)
		}
		for _, dependency := range component.DependsOn {
			if _, selected := byID[dependency]; selected {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		state[id] = visited
		ordered = append(ordered, byID[id])
		return nil
	}
	for _, action := range actions {
		if err := visit(action.ComponentID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func tierRank(tier model.Tier) int {
	switch tier {
	case model.TierEssential:
		return 0
	case model.TierRecommended:
		return 1
	case model.TierOptionalLocal:
		return 2
	case model.TierCredential:
		return 3
	default:
		return 4
	}
}
