// Package strictjson decodes security-sensitive JSON documents without
// accepting unknown fields or trailing content.
package strictjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode parses exactly one JSON value into destination.
func Decode(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON content")
		}
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}
