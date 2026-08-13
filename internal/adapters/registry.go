package adapters

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	items     map[string]Adapter
	canonical map[string]Adapter
}

func NewRegistry(values ...Adapter) (*Registry, error) {
	registry := &Registry{items: map[string]Adapter{}, canonical: map[string]Adapter{}}
	for _, adapter := range values {
		if adapter == nil {
			return nil, fmt.Errorf("nil adapter")
		}
		id := strings.TrimSpace(adapter.ID())
		if id == "" || strings.TrimSpace(adapter.SchemaVersion()) == "" {
			return nil, fmt.Errorf("adapter identity is empty")
		}
		if adapter.SchemaVersion() != ContractVersion {
			return nil, fmt.Errorf("adapter %q uses unsupported contract %q", id, adapter.SchemaVersion())
		}
		if _, exists := registry.canonical[id]; exists {
			return nil, fmt.Errorf("duplicate adapter %q", id)
		}
		registry.canonical[id] = adapter
		registry.items[id] = adapter
	}
	for id, adapter := range registry.canonical {
		provider, ok := adapter.(interface{ Aliases() []string })
		if !ok {
			continue
		}
		for _, alias := range provider.Aliases() {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == id {
				continue
			}
			if _, canonical := registry.canonical[alias]; canonical {
				return nil, fmt.Errorf("adapter alias %q collides with a canonical id", alias)
			}
			if _, exists := registry.items[alias]; exists {
				return nil, fmt.Errorf("duplicate adapter alias %q", alias)
			}
			registry.items[alias] = adapter
		}
	}
	return registry, nil
}

func (r *Registry) Get(id string) (Adapter, error) {
	if r == nil {
		return nil, fmt.Errorf("adapter registry is unavailable")
	}
	adapter, ok := r.items[strings.TrimSpace(id)]
	if !ok {
		return nil, fmt.Errorf("unsupported adapter target %q", id)
	}
	return adapter, nil
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.canonical))
	for id := range r.canonical {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) Capabilities(ctx context.Context, environment Environment, ids []string) ([]CapabilitySet, error) {
	if len(ids) == 0 {
		ids = r.IDs()
	}
	seenTargets := map[string]struct{}{}
	result := make([]CapabilitySet, 0, len(ids))
	for _, id := range ids {
		adapter, err := r.Get(id)
		if err != nil {
			return nil, err
		}
		capability, err := adapter.Capabilities(ctx, environment)
		if err != nil {
			return nil, err
		}
		if err := VerifyCapabilityForAdapter(adapter, capability); err != nil {
			return nil, err
		}
		if _, duplicate := seenTargets[capability.Target]; duplicate {
			continue
		}
		seenTargets[capability.Target] = struct{}{}
		result = append(result, capability)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Target < result[j].Target })
	return result, nil
}
