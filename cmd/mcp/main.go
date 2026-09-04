// Command nudgeway-mcp is a standalone Model Context Protocol (MCP) server
// that exposes every operation in internal/api/openapi/openapi.yaml as an
// MCP tool. It speaks JSON-RPC 2.0 over stdio using MCP protocol version
// 2024-11-05.
//
// Environment variables:
//
//	NUDGEWAY_API_URL         Base URL of the running Nudgeway server.
//	                         Defaults to http://127.0.0.1:8080.
//	NUDGEWAY_SESSION_COOKIE  Value of the `nudgeway_session` cookie.
//	NUDGEWAY_CSRF_TOKEN      Optional CSRF token for state-changing calls.
//
// Flags:
//
//	--list-tools             Print the tool list as JSON to stdout and exit.
//	                         Useful for debugging without an MCP client.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/v-senthil/nudgeway/internal/api/openapi"
	"github.com/v-senthil/nudgeway/internal/mcp"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "nudgeway-mcp"
	serverVersion   = "0.1.0"
)

// jsonrpcRequest is the minimal shape of an inbound JSON-RPC 2.0 message.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is the outbound success or error envelope.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError carries an RPC failure back to the client.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func main() {
	listTools := flag.Bool("list-tools", false, "Print the generated tool list as JSON and exit.")
	flag.Parse()

	tools, err := mcp.LoadTools(openapi.SpecYAML())
	if err != nil {
		fmt.Fprintf(os.Stderr, "nudgeway-mcp: load tools: %v\n", err)
		os.Exit(1)
	}

	if *listTools {
		b, err := mcp.ToolsAsJSON(tools)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nudgeway-mcp: marshal tools: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	baseURL := getenvDefault("NUDGEWAY_API_URL", "http://127.0.0.1:8080")
	forwarder := &mcp.Forwarder{
		BaseURL:       baseURL,
		SessionCookie: os.Getenv("NUDGEWAY_SESSION_COOKIE"),
		CSRFToken:     os.Getenv("NUDGEWAY_CSRF_TOKEN"),
	}

	toolIndex := make(map[string]mcp.Tool, len(tools))
	for _, t := range tools {
		toolIndex[t.Name] = t
	}

	srv := &server{
		tools:     tools,
		toolIndex: toolIndex,
		forward:   forwarder,
		writer:    bufio.NewWriter(os.Stdout),
	}
	if err := srv.run(context.Background(), os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "nudgeway-mcp: %v\n", err)
		os.Exit(1)
	}
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// server holds the running MCP server state.
type server struct {
	tools     []mcp.Tool
	toolIndex map[string]mcp.Tool
	forward   *mcp.Forwarder

	writeMu sync.Mutex
	writer  *bufio.Writer
}

// run reads JSON-RPC messages one per line from r and dispatches them.
// The MCP stdio framing is newline-delimited JSON.
func (s *server) run(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Allow up to 4 MiB per line for large tool arguments.
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		// Copy — scanner reuses its buffer between calls.
		payload := append([]byte(nil), line...)
		if err := s.handle(ctx, payload); err != nil {
			fmt.Fprintf(os.Stderr, "nudgeway-mcp: handle: %v\n", err)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("stdin scan: %w", err)
	}
	return nil
}

// handle routes a single JSON-RPC message to the right handler.
func (s *server) handle(ctx context.Context, payload []byte) error {
	var req jsonrpcRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return s.writeError(nil, -32700, fmt.Sprintf("parse error: %v", err))
	}

	// Notifications (no id) never receive a response.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return s.writeResult(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
			},
			"serverInfo": map[string]any{
				"name":    serverName,
				"version": serverVersion,
			},
		})

	case "ping":
		return s.writeResult(req.ID, map[string]any{})

	case "tools/list":
		return s.writeResult(req.ID, map[string]any{
			"tools": s.tools,
		})

	case "tools/call":
		return s.handleToolsCall(ctx, req.ID, req.Params)

	case "notifications/initialized",
		"notifications/cancelled",
		"notifications/progress",
		"notifications/roots/list_changed":
		// Notifications carry no reply.
		return nil

	default:
		if isNotification {
			return nil
		}
		return s.writeError(req.ID, -32601, "method not found: "+req.Method)
	}
}

// toolsCallParams matches the MCP `tools/call` request shape.
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (s *server) handleToolsCall(ctx context.Context, id json.RawMessage, raw json.RawMessage) error {
	var p toolsCallParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return s.writeError(id, -32602, fmt.Sprintf("invalid params: %v", err))
	}
	tool, ok := s.toolIndex[p.Name]
	if !ok {
		return s.writeToolError(id, fmt.Sprintf("unknown tool: %s", p.Name))
	}

	res, err := s.forward.Forward(ctx, tool, p.Arguments)
	if err != nil {
		return s.writeToolError(id, err.Error())
	}

	isError := res.StatusCode < 200 || res.StatusCode >= 300
	return s.writeResult(id, map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": mcp.RenderText(res),
			},
		},
		"isError": isError,
	})
}

// writeResult emits a JSON-RPC success envelope.
func (s *server) writeResult(id json.RawMessage, result any) error {
	return s.writeMessage(jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

// writeError emits a JSON-RPC error envelope.
func (s *server) writeError(id json.RawMessage, code int, msg string) error {
	return s.writeMessage(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonrpcError{Code: code, Message: msg},
	})
}

// writeToolError wraps a tool-call failure into the MCP `isError=true`
// content shape rather than a JSON-RPC protocol error, matching the MCP
// spec's guidance that tool failures are surfaced in-band.
func (s *server) writeToolError(id json.RawMessage, msg string) error {
	return s.writeResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	})
}

// writeMessage serialises msg as a single newline-delimited JSON line.
func (s *server) writeMessage(msg jsonrpcResponse) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.writer.Write(b); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}
