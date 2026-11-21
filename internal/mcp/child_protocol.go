package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agentstack/agentstack/internal/redact"
)

type childResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

func writeJSONLine(writer io.Writer, value any, limit int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("MCP message exceeds %d-byte limit", limit)
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}

func readResultLine(reader *bufio.Reader, expected int, limit int) (json.RawMessage, error) {
	const maxMismatchedResponses = 128
	mismatched := 0
	for {
		line, err := readLimitedLine(reader, limit)
		if err != nil {
			return nil, err
		}
		var response childResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return nil, fmt.Errorf("decode child JSON-RPC response: %w", err)
		}
		var id int
		if len(response.ID) == 0 || json.Unmarshal(response.ID, &id) != nil || id != expected {
			mismatched++
			if mismatched > maxMismatchedResponses {
				return nil, fmt.Errorf("child returned %d mismatched responses without matching id %d", mismatched, expected)
			}
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("child JSON-RPC error %d: %s", response.Error.Code, response.Error.Message)
		}
		if len(response.Result) == 0 {
			return nil, fmt.Errorf("child response has no result")
		}
		return response.Result, nil
	}
}

func readLimitedLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var result []byte
	for {
		fragment, prefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if len(result)+len(fragment) > limit {
			return nil, fmt.Errorf("MCP message exceeds %d-byte limit", limit)
		}
		result = append(result, fragment...)
		if !prefix {
			return result, nil
		}
	}
}

func supportedProtocol(version string) bool {
	for _, candidate := range SupportedProtocolVersions {
		if version == candidate {
			return true
		}
	}
	return false
}

func childError(err error, stderr string, truncated bool) error {
	stderr = redactChildError(stderr)
	if stderr == "" {
		return err
	}
	if truncated {
		stderr += " (truncated)"
	}
	return fmt.Errorf("%w; child stderr: %s", err, stderr)
}

func redactChildError(value string) string {
	value = redact.Text(strings.TrimSpace(value))
	if len(value) > defaultStderrLimit {
		value = value[:defaultStderrLimit] + "…"
	}
	return value
}
