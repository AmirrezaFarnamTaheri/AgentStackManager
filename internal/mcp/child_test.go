package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChildObserverEmitsMinimizedLifecycleEvents(t *testing.T) {
	var mu sync.Mutex
	var events []ChildEvent
	observer := func(event ChildEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	client := StdIOChildClient{Timeout: 5 * time.Second, Observer: observer}
	server := ServerConfig{Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--", "secret-argument"}, Env: map[string]string{"GO_WANT_MCP_HELPER": "1", "API_TOKEN": "do-not-log"}}
	if _, err := client.ListTools(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
		serialized, _ := json.Marshal(event)
		if strings.Contains(string(serialized), "secret-argument") || strings.Contains(string(serialized), "do-not-log") {
			t.Fatalf("observer leaked child inputs: %s", serialized)
		}
		if event.ServerKey == "" || event.Command == "" {
			t.Fatalf("observer event lacks stable identity: %+v", event)
		}
	}
	for _, kind := range []string{"child.launch", "child.request", "child.stop"} {
		if !seen[kind] {
			t.Fatalf("missing %s event: %+v", kind, events)
		}
	}
}

func TestStdIOChildClientListToolsAndCall(t *testing.T) {
	client := StdIOChildClient{Timeout: 5 * time.Second}
	server := ServerConfig{Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--"}, Env: map[string]string{"GO_WANT_MCP_HELPER": "1"}}
	listed, err := client.ListTools(context.Background(), server)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(listed), "helper_tool") {
		t.Fatalf("unexpected list result: %s", listed)
	}
	called, err := client.CallTool(context.Background(), server, "helper_tool", json.RawMessage(`{"value":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(called), "value=7") {
		t.Fatalf("unexpected call result: %s", called)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	if countFile := os.Getenv("MCP_HELPER_COUNT_FILE"); countFile != "" {
		file, _ := os.OpenFile(countFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			_, _ = file.WriteString("start\n")
			_ = file.Close()
		}
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request map[string]any
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		method, _ := request["method"].(string)
		id, hasID := request["id"]
		if !hasID {
			continue
		}
		switch method {
		case "initialize":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "helper", "version": "1"}}})
		case "tools/list":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{map[string]any{"name": "helper_tool", "inputSchema": map[string]any{"type": "object"}}}}})
		case "tools/call":
			params := request["params"].(map[string]any)
			args := params["arguments"].(map[string]any)
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("value=%v", args["value"])}}}})
		}
	}
}

func TestPooledChildClientReusesPersistentServerAndClosesIt(t *testing.T) {
	countFile := t.TempDir() + string(os.PathSeparator) + "starts.txt"
	server := ServerConfig{
		Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:        map[string]string{"GO_WANT_MCP_HELPER": "1", "MCP_HELPER_COUNT_FILE": countFile},
		Persistent: true, IdleTTLSeconds: 60,
	}
	client := &PooledChildClient{Base: StdIOChildClient{Timeout: 5 * time.Second}}
	for index := 0; index < 3; index++ {
		if _, err := client.ListTools(context.Background(), server); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "start\n" {
		t.Fatalf("persistent child was restarted: %q", data)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkChildListTools(b *testing.B) {
	server := ServerConfig{Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--"}, Env: map[string]string{"GO_WANT_MCP_HELPER": "1"}}
	b.Run("one-shot", func(b *testing.B) {
		client := StdIOChildClient{Timeout: 5 * time.Second}
		for index := 0; index < b.N; index++ {
			if _, err := client.ListTools(context.Background(), server); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("persistent", func(b *testing.B) {
		server.Persistent = true
		client := &PooledChildClient{Base: StdIOChildClient{Timeout: 5 * time.Second}, IdleTTL: time.Minute}
		defer client.Close()
		for index := 0; index < b.N; index++ {
			if _, err := client.ListTools(context.Background(), server); err != nil {
				b.Fatal(err)
			}
		}
	})
}
