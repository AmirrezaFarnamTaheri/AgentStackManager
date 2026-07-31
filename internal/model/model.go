package model

import "time"

type Tier string

const (
	TierEssential     Tier = "essential"
	TierRecommended   Tier = "recommended"
	TierOptionalLocal Tier = "optional-local"
	TierCredential    Tier = "credential"
)

type InstallKind string

const (
	InstallNone      InstallKind = "none"
	InstallWinget    InstallKind = "winget"
	InstallNPMGlobal InstallKind = "npm-global"
	InstallUVTool    InstallKind = "uv-tool"
	InstallRouter    InstallKind = "router"
	InstallSkillPack InstallKind = "skill-pack"
	InstallManual    InstallKind = "manual"
)

type CommandSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type InstallSpec struct {
	Kind               InstallKind `json:"kind"`
	Package            string      `json:"package,omitempty"`
	WingetID           string      `json:"wingetId,omitempty"`
	Repository         string      `json:"repository,omitempty"`
	RepositoryRevision string      `json:"repositoryRevision,omitempty"`
	ManifestDigest     string      `json:"manifestDigest,omitempty"`
	Source             string      `json:"source,omitempty"`
	Version            string      `json:"version,omitempty"`
	Publisher          string      `json:"publisher,omitempty"`
	Digest             string      `json:"digest,omitempty"`
	ExpectedEntries    []string    `json:"expectedEntries,omitempty"`
	LoginHint          string      `json:"loginHint,omitempty"`
	DocumentationURL   string      `json:"documentationUrl,omitempty"`
	Description        string      `json:"description,omitempty"`
}

type VersionPolicy struct {
	Minimum string      `json:"minimum,omitempty"`
	Maximum string      `json:"maximum,omitempty"`
	Probe   CommandSpec `json:"probe"`
	Pattern string      `json:"pattern,omitempty"`
}

type ProcessLimits struct {
	MemoryBytes     uint64 `json:"memoryBytes,omitempty"`
	CPUPercent      uint32 `json:"cpuPercent,omitempty"`
	ActiveProcesses uint32 `json:"activeProcesses,omitempty"`
}

type RouterServerSpec struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Warm           *CommandSpec      `json:"warm,omitempty"`
	Persistent     bool              `json:"persistent,omitempty"`
	IdleTTLSeconds int               `json:"idleTTLSeconds,omitempty"`
	Limits         ProcessLimits     `json:"limits,omitempty"`
}

type Component struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Tier               Tier              `json:"tier"`
	Category           string            `json:"category"`
	Capability         string            `json:"capability,omitempty"`
	Preferred          bool              `json:"preferred,omitempty"`
	CredentialRequired bool              `json:"credentialRequired,omitempty"`
	DetectCommands     []string          `json:"detectCommands,omitempty"`
	DependsOn          []string          `json:"dependsOn,omitempty"`
	Install            InstallSpec       `json:"install"`
	Router             *RouterServerSpec `json:"router,omitempty"`
	VersionPolicy      *VersionPolicy    `json:"versionPolicy,omitempty"`
	Platforms          []string          `json:"platforms,omitempty"`
	GuidedSetup        bool              `json:"guidedSetup,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
}

type Profile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Components  []string `json:"components"`
}

type Catalog struct {
	Version    int         `json:"version"`
	Components []Component `json:"components"`
	Profiles   []Profile   `json:"profiles"`
}

func (c Catalog) ComponentByID(id string) (Component, bool) {
	for _, component := range c.Components {
		if component.ID == id {
			return component, true
		}
	}
	return Component{}, false
}

func (c Catalog) ProfileByID(id string) (Profile, bool) {
	for _, profile := range c.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}

type InventoryItem struct {
	ComponentID     string `json:"componentId"`
	Installed       bool   `json:"installed"`
	DetectedCommand string `json:"detectedCommand,omitempty"`
	ExecutablePath  string `json:"executablePath,omitempty"`
	Version         string `json:"version,omitempty"`
	Compatible      bool   `json:"compatible,omitempty"`
	Incompatible    bool   `json:"incompatible,omitempty"`
	PackageSource   string `json:"packageSource,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	Managed         bool   `json:"managed,omitempty"`
	Broken          bool   `json:"broken,omitempty"`
	HealthMessage   string `json:"healthMessage,omitempty"`
}

type ExternalPackage struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	ID        string `json:"id,omitempty"`
	Source    string `json:"source,omitempty"`
	Publisher string `json:"publisher,omitempty"`
}

type Inventory struct {
	GeneratedAt time.Time                    `json:"generatedAt"`
	Revision    string                       `json:"revision"`
	Items       map[string]InventoryItem     `json:"items"`
	External    map[string][]ExternalPackage `json:"external,omitempty"`
	RawSources  map[string]string            `json:"rawSources,omitempty"`
}

type ActionKind string

const (
	ActionKeep             ActionKind = "keep"
	ActionInstall          ActionKind = "install"
	ActionRepair           ActionKind = "repair"
	ActionConfigure        ActionKind = "configure"
	ActionPreserveInactive ActionKind = "preserve-inactive"
	ActionConsentRequired  ActionKind = "consent-required"
	ActionSkipDominated    ActionKind = "skip-dominated"
	ActionSkip             ActionKind = "skip"
)

type PlanAction struct {
	ComponentID string      `json:"componentId"`
	Name        string      `json:"name"`
	Kind        ActionKind  `json:"kind"`
	Reason      string      `json:"reason"`
	Install     InstallSpec `json:"install"`
	Selected    bool        `json:"selected"`
	Upgrade     bool        `json:"upgrade,omitempty"`
}

type Plan struct {
	ID            string            `json:"id"`
	Digest        string            `json:"digest"`
	Profile       string            `json:"profile"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	CatalogHash   string            `json:"catalogHash"`
	InventoryHash string            `json:"inventoryHash"`
	Actions       []PlanAction      `json:"actions"`
	Providers     map[string]string `json:"providers,omitempty"`
}

func (p Plan) ActionFor(componentID string) PlanAction {
	for _, action := range p.Actions {
		if action.ComponentID == componentID {
			return action
		}
	}
	return PlanAction{ComponentID: componentID, Kind: ActionSkip, Reason: "not selected"}
}

type TransactionStatus string

const (
	TransactionPlanned     TransactionStatus = "planned"
	TransactionRunning     TransactionStatus = "running"
	TransactionSucceeded   TransactionStatus = "succeeded"
	TransactionFailed      TransactionStatus = "failed"
	TransactionInterrupted TransactionStatus = "interrupted"
	TransactionPartial     TransactionStatus = "partial"
)

type TransactionAction struct {
	ComponentID     string     `json:"componentId"`
	Kind            ActionKind `json:"kind"`
	Command         string     `json:"command,omitempty"`
	Args            []string   `json:"args,omitempty"`
	ExitCode        int        `json:"exitCode,omitempty"`
	Output          string     `json:"output,omitempty"`
	OutputTruncated bool       `json:"outputTruncated,omitempty"`
	Verified        bool       `json:"verified,omitempty"`
	Verification    string     `json:"verification,omitempty"`
	Error           string     `json:"error,omitempty"`
	StartedAt       time.Time  `json:"startedAt,omitempty"`
	FinishedAt      time.Time  `json:"finishedAt,omitempty"`
}

type Transaction struct {
	ID            string              `json:"id"`
	CorrelationID string              `json:"correlationId,omitempty"`
	Status        TransactionStatus   `json:"status"`
	StartedAt     time.Time           `json:"startedAt"`
	FinishedAt    time.Time           `json:"finishedAt,omitempty"`
	DryRun        bool                `json:"dryRun"`
	Actions       []TransactionAction `json:"actions"`
}
