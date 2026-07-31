package integrity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// DigestJSON returns a deterministic SHA-256 digest of a JSON-serializable value.
// encoding/json sorts map keys, making this suitable for catalog, inventory, and
// reviewed-plan identity checks.
func DigestJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal digest input: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
