package mcp

import (
	"encoding/json"
	"testing"
)

func FuzzRouterConfigJSON(f *testing.F) {
	f.Add([]byte(`{"version":1,"servers":{}}`))
	f.Add([]byte(`{"version":0}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var config RouterConfig
		if json.Unmarshal(data, &config) == nil {
			_ = RouterConfigEquivalent(config, config)
			_ = SortedServerNames(config)
		}
	})
}

func FuzzRegistrationMap(f *testing.F) {
	f.Add("agentstack", "mcp-router,--config,C:/x.json")
	f.Fuzz(func(t *testing.T, command, joined string) {
		args := []any{}
		for _, value := range splitFuzz(joined) {
			args = append(args, value)
		}
		_, _ = parseRegistrationMap(map[string]any{"command": command, "args": args})
	})
}

func splitFuzz(value string) []string {
	var out []string
	start := 0
	for i, r := range value {
		if r == ',' {
			out = append(out, value[start:i])
			start = i + 1
		}
	}
	return append(out, value[start:])
}
