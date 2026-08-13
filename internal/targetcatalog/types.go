package targetcatalog

import (
	"fmt"
	"sort"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const APIVersion = "targetcatalog.asm.dev/v1alpha1"

type ConfidenceLevel string

const (
	ConfidenceHigh       ConfidenceLevel = "high"
	ConfidenceMedium     ConfidenceLevel = "medium"
	ConfidenceLow        ConfidenceLevel = "low"
	ConfidenceUnverified ConfidenceLevel = "unverified"
)

type TargetScope string

const (
	ScopeGlobal  TargetScope = "global"
	ScopeProject TargetScope = "project"
)

type TargetDefinition struct {
	ID                 string             `json:"id"`
	DisplayName        string             `json:"displayName"`
	Vendor             string             `json:"vendor"`
	Scope              TargetScope        `json:"scope"`
	Aliases            []string           `json:"aliases,omitempty"`
	GlobalRootPattern  string             `json:"globalRootPattern,omitempty"`
	ProjectRootPattern string             `json:"projectRootPattern,omitempty"`
	Subpath            string             `json:"subpath,omitempty"`
	DiscoveryOnlyPaths []string           `json:"discoveryOnlyPaths,omitempty"`
	SupportedKinds     []resourcehub.Kind `json:"supportedKinds"`
	OwnershipMarkers   []string           `json:"ownershipMarkers,omitempty"`
	RecursionAllowed   bool               `json:"recursionAllowed"`
	ReadOnly           bool               `json:"readOnly"`
	Confidence         ConfidenceLevel    `json:"confidence"`
}

type CatalogSchema struct {
	APIVersion string             `json:"apiVersion"`
	Targets    []TargetDefinition `json:"targets"`
	Digest     string             `json:"digest"`
}

type DetectedTarget struct {
	Definition TargetDefinition `json:"definition"`
	RootPath   string           `json:"rootPath"`
	FoundPath  string           `json:"foundPath"`
	IsProject  bool             `json:"isProject"`
	Managed    bool             `json:"managed"`
}

// SealCatalogSchema normalizes target definitions and computes a SHA-256 digest.
func SealCatalogSchema(cat CatalogSchema) (CatalogSchema, error) {
	if cat.APIVersion == "" {
		cat.APIVersion = APIVersion
	}
	if cat.Targets == nil {
		cat.Targets = []TargetDefinition{}
	} else {
		for i := range cat.Targets {
			if cat.Targets[i].Aliases == nil {
				cat.Targets[i].Aliases = []string{}
			} else {
				sort.Strings(cat.Targets[i].Aliases)
			}
			if cat.Targets[i].DiscoveryOnlyPaths == nil {
				cat.Targets[i].DiscoveryOnlyPaths = []string{}
			} else {
				sort.Strings(cat.Targets[i].DiscoveryOnlyPaths)
			}
			if cat.Targets[i].OwnershipMarkers == nil {
				cat.Targets[i].OwnershipMarkers = []string{}
			} else {
				sort.Strings(cat.Targets[i].OwnershipMarkers)
			}
		}
		sort.Slice(cat.Targets, func(i, j int) bool {
			return cat.Targets[i].ID < cat.Targets[j].ID
		})
	}

	cat.Digest = ""
	digest, err := integrity.DigestJSON(cat)
	if err != nil {
		return CatalogSchema{}, fmt.Errorf("digest target catalog: %w", err)
	}
	cat.Digest = digest
	return cat, nil
}

// VerifyCatalogSchema verifies digest against SealCatalogSchema.
func VerifyCatalogSchema(cat CatalogSchema) error {
	if cat.APIVersion != APIVersion {
		return fmt.Errorf("unsupported target catalog API version %q", cat.APIVersion)
	}
	if cat.Digest == "" {
		return fmt.Errorf("target catalog digest is empty")
	}
	expected, err := SealCatalogSchema(cat)
	if err != nil {
		return err
	}
	if expected.Digest != cat.Digest {
		return fmt.Errorf("target catalog digest mismatch: got %q, expected %q", cat.Digest, expected.Digest)
	}
	return nil
}

// UnmarshalCatalogSchemaJSON decodes and verifies a catalog schema from JSON bytes.
func UnmarshalCatalogSchemaJSON(data []byte) (CatalogSchema, error) {
	var cat CatalogSchema
	if err := strictjson.Decode(data, &cat); err != nil {
		return CatalogSchema{}, fmt.Errorf("decode target catalog: %w", err)
	}
	if cat.APIVersion != APIVersion {
		return CatalogSchema{}, fmt.Errorf("unsupported target catalog API version %q", cat.APIVersion)
	}
	if err := VerifyCatalogSchema(cat); err != nil {
		return CatalogSchema{}, fmt.Errorf("verify target catalog: %w", err)
	}
	return cat, nil
}
