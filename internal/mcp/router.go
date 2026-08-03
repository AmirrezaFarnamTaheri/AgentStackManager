package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type Router struct {
	Config          RouterConfig
	Children        ChildClient
	MaxMessageBytes int
}

type routerSession struct {
	initialized     bool
	protocolVersion string
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (r Router) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if r.Children == nil {
		r.Children = NewManagedChildRuntime(ChildRuntimeOptions{})
	}
	if closer, ok := r.Children.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	limit := r.MaxMessageBytes
	if limit <= 0 {
		limit = defaultMessageLimit
	}
	reader := bufio.NewReaderSize(input, 64*1024)
	encoder := json.NewEncoder(output)
	session := &routerSession{}
	for {
		line, err := readLimitedLine(reader, limit)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode MCP request: %w", err)
		}
		var request rpcRequest
		if err := json.Unmarshal(line, &request); err != nil {
			return fmt.Errorf("decode MCP request: %w", err)
		}
		if len(request.ID) == 0 {
			if request.Method == "notifications/initialized" {
				session.initialized = true
			}
			continue
		}
		response := r.handle(ctx, session, request)
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode MCP response: %w", err)
		}
	}
}

func (r Router) handle(ctx context.Context, session *routerSession, request rpcRequest) rpcResponse {
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			response.Error = &rpcError{Code: -32602, Message: "invalid initialize parameters"}
			return response
		}
		selected := negotiateProtocol(params.ProtocolVersion)
		session.protocolVersion = selected
		response.Result = map[string]any{
			"protocolVersion": selected,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "agentstack-router", "version": "1.0.0"},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		if !session.initialized {
			response.Error = &rpcError{Code: -32002, Message: "MCP session is not initialized"}
			return response
		}
		response.Result = map[string]any{"tools": routerTools()}
	case "tools/call":
		if !session.initialized {
			response.Error = &rpcError{Code: -32002, Message: "MCP session is not initialized"}
			return response
		}
		result, err := r.callTool(ctx, request.Params)
		if err != nil {
			response.Result = errorToolResult(err.Error())
		} else {
			response.Result = result
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return response
}

func negotiateProtocol(requested string) string {
	if supportedProtocol(requested) {
		return requested
	}
	return SupportedProtocolVersions[0]
}

func routerTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "agentstack_router_list_servers",
			"description": "List MCP child servers available through the AgentStack router.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		},
		{
			"name":        "agentstack_router_list_tools",
			"description": "Lazily start one child MCP server and list its tools.",
			"inputSchema": objectSchema(map[string]any{"server": stringProperty("Child server name")}, []string{"server"}),
		},
		{
			"name":        "agentstack_router_call_tool",
			"description": "Call one tool on one lazily started child MCP server.",
			"inputSchema": objectSchema(map[string]any{
				"server":    stringProperty("Child server name"),
				"tool":      stringProperty("Child tool name"),
				"arguments": map[string]any{"type": "object", "description": "Arguments passed to the child tool", "additionalProperties": true},
			}, []string{"server", "tool"}),
		},
		{
			"name":        "agentstack_router_doctor",
			"description": "Check whether configured child MCP commands are available.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description, "minLength": 1}
}

func (r Router) callTool(ctx context.Context, raw json.RawMessage) (any, error) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("decode tools/call params: %w", err)
	}
	switch params.Name {
	case "agentstack_router_list_servers":
		return textToolResult(mustJSON(map[string]any{"servers": SortedServerNames(r.Config)})), nil
	case "agentstack_router_list_tools":
		var args struct {
			Server string `json:"server"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("decode list-tools arguments: %w", err)
		}
		server, ok := r.Config.Servers[args.Server]
		if !ok {
			return nil, fmt.Errorf("unknown child server %q", args.Server)
		}
		result, err := r.Children.ListTools(ctx, server)
		if err != nil {
			return nil, err
		}
		return textToolResult(string(result)), nil
	case "agentstack_router_call_tool":
		var args struct {
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("decode call-tool arguments: %w", err)
		}
		server, ok := r.Config.Servers[args.Server]
		if !ok {
			return nil, fmt.Errorf("unknown child server %q", args.Server)
		}
		result, err := r.Children.CallTool(ctx, server, args.Tool, args.Arguments)
		if err != nil {
			return nil, err
		}
		var decoded any
		if err := json.Unmarshal(result, &decoded); err != nil {
			return nil, fmt.Errorf("decode child call result: %w", err)
		}
		return decoded, nil
	case "agentstack_router_doctor":
		names := SortedServerNames(r.Config)
		checks := make(map[string]DoctorItem, len(names))
		for _, name := range names {
			checks[name] = r.Children.Doctor(ctx, r.Config.Servers[name])
		}
		return textToolResult(mustJSON(map[string]any{"servers": checks})), nil
	default:
		return nil, fmt.Errorf("unknown router tool %q", params.Name)
	}
}

func textToolResult(text string) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
}

func errorToolResult(message string) map[string]any {
	result := textToolResult(message)
	result["isError"] = true
	return result
}
func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `{"error":"encode failure"}`
	}
	return string(data)
}
