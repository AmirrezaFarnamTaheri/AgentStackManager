package app

import (
	"context"
	"sort"

	"github.com/agentstack/agentstack/internal/model"
)

type GuidedIntegration struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Installed         bool              `json:"installed"`
	InstallKind       model.InstallKind `json:"installKind"`
	LoginHint         string            `json:"loginHint"`
	AgentStoresSecret bool              `json:"agentStoresSecret"`
	NextStep          string            `json:"nextStep"`
	DocumentationURL  string            `json:"documentationUrl"`
	Verification      string            `json:"verification"`
}

func (s *Service) GuidedIntegrations(ctx context.Context) ([]GuidedIntegration, error) {
	current, err := s.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	result := []GuidedIntegration{}
	for _, component := range s.Catalog.Components {
		if !component.CredentialRequired {
			continue
		}
		item := current.Items[component.ID]
		next := component.Install.LoginHint
		if next == "" {
			next = "Follow the provider's official authentication flow."
		}
		verification := "Re-run agentstack integrations and the provider's official identity/status command."
		if len(component.DetectCommands) > 0 {
			verification = "Confirm " + component.DetectCommands[0] + " is available, then run the provider's official auth status command."
		}
		result = append(result, GuidedIntegration{
			ID: component.ID, Name: component.Name, Description: component.Description,
			Installed: item.Installed, InstallKind: component.Install.Kind,
			LoginHint: component.Install.LoginHint, AgentStoresSecret: false,
			NextStep: next, DocumentationURL: component.Install.DocumentationURL, Verification: verification,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
