package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeChild struct {
	listed string
	called string
}

func (f *fakeChild) ListTools(context.Context, ServerConfig) (json.RawMessage, error) {
	f.listed = "yes"
	return json.RawMessage(`{"tools":[{"name":"child_tool","inputSchema":{"type":"object"}}]}`), nil
}
func (f *fakeChild) CallTool(_ context.Context, _ ServerConfig, name string, arguments json.RawMessage) (json.RawMessage, error) {
	f.called = name + ":" + string(arguments)
	return json.RawMessage(`{"content":[{"type":"text","text":"child result"}]}`), nil
}
func (f *fakeChild) Doctor(context.Context, ServerConfig) DoctorItem { return DoctorItem{Status: "ok"} }

func TestRouterInitializeAndToolsList(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		"",
	}, "\n")
	var output bytes.Buffer
	router := Router{Config: RouterConfig{Version: 1, Servers: map[string]ServerConfig{}}, Children: &fakeChild{}}
	if err := router.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two responses, got %d: %s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"serverInfo"`) {
		t.Fatalf("initialize response missing serverInfo: %s", lines[0])
	}
	if !strings.Contains(lines[1], `agentstack_router_call_tool`) {
		t.Fatalf("tools list missing router tool: %s", lines[1])
	}
}

func TestRouterForwardsListAndCallToSelectedChild(t *testing.T) {
	child := &fakeChild{}
	config := RouterConfig{Version: 1, Servers: map[string]ServerConfig{"memory": {Command: "fake"}}}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agentstack_router_list_tools","arguments":{"server":"memory"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agentstack_router_call_tool","arguments":{"server":"memory","tool":"child_tool","arguments":{"x":1}}}}`,
		"",
	}, "\n")
	var output bytes.Buffer
	router := Router{Config: config, Children: child}
	if err := router.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if child.listed != "yes" {
		t.Fatal("child list not called")
	}
	if child.called != `child_tool:{"x":1}` {
		t.Fatalf("unexpected call %q", child.called)
	}
	if !strings.Contains(output.String(), "child result") {
		t.Fatalf("child response not forwarded: %s", output.String())
	}
}

func TestRouterRejectsUnknownServerWithoutCallingChild(t *testing.T) {
	child := &fakeChild{}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agentstack_router_list_tools","arguments":{"server":"missing"}}}`,
		"",
	}, "\n")
	var output bytes.Buffer
	router := Router{Config: RouterConfig{Version: 1, Servers: map[string]ServerConfig{}}, Children: child}
	if err := router.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"isError":true`) {
		t.Fatalf("expected tool error: %s", output.String())
	}
	if child.listed != "" {
		t.Fatal("child should not be called")
	}
}

func TestRouterNegotiatesSupportedProtocol(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1900-01-01"}}` + "\n"
	var output bytes.Buffer
	router := Router{Config: RouterConfig{Version: 1, Servers: map[string]ServerConfig{}}, Children: &fakeChild{}}
	if err := router.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("router did not negotiate a supported version: %s", output.String())
	}
}

func TestRouterRejectsToolsBeforeInitializedNotification(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		"",
	}, "\n")
	var output bytes.Buffer
	router := Router{Config: RouterConfig{Version: 1, Servers: map[string]ServerConfig{}}, Children: &fakeChild{}}
	if err := router.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"code":-32002`) {
		t.Fatalf("expected initialization error: %s", output.String())
	}
}
