package donormanifest

const (
	SchemaVersion = 1

	ObjectTypeFile      = "file"
	ObjectTypeDir       = "directory"
	ObjectTypeSymlink   = "symlink"
	ObjectTypeReparse   = "reparse"
	ObjectTypeUnknown   = "unknown"
	ObjectTypeReadError = "error"

	DispositionUnreviewed = "unreviewed"
)

// DispositionRule maps a path pattern to a donor disposition, destination, and reason.
type DispositionRule struct {
	Pattern     string `json:"path"`
	Disposition string `json:"disposition"`
	Destination string `json:"destination,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Manifest represents a deterministic inventory of a donor snapshot.
type Manifest struct {
	SchemaVersion    int             `json:"schemaVersion"`
	SnapshotID       string          `json:"snapshotId"`
	SourceRoot       string          `json:"sourceRoot"`
	PhysicalIdentity string          `json:"physicalIdentity"`
	InventoryTime    string          `json:"inventoryTime"`
	EmptySnapshot    bool            `json:"emptySnapshot"`
	FileCount        int             `json:"fileCount"`
	TotalSizeBytes   int64           `json:"totalSizeBytes"`
	Entries          []ManifestEntry `json:"entries"`
}

// ManifestEntry represents a single file/directory/symlink in the snapshot.
type ManifestEntry struct {
	Path          string `json:"path"`
	ObjectType    string `json:"objectType"`
	SizeBytes     int64  `json:"sizeBytes"`
	SHA256        string `json:"sha256,omitempty"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
	ReadError     string `json:"readError,omitempty"`
	Disposition   string `json:"disposition"`
	Destination   string `json:"destination,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// RootReceipt is a compact, sealed proof of snapshot inventory.
type RootReceipt struct {
	SchemaVersion    int    `json:"schemaVersion"`
	SnapshotID       string `json:"snapshotId"`
	SourceRoot       string `json:"sourceRoot"`
	ManifestDigest   string `json:"manifestDigest"`
	PhysicalIdentity string `json:"physicalIdentity"`
	InventoryTime    string `json:"inventoryTime"`
	EmptySnapshot    bool   `json:"emptySnapshot"`
	FileCount        int    `json:"fileCount"`
	TotalSizeBytes   int64  `json:"totalSizeBytes"`
}
