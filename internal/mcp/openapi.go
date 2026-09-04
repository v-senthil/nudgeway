// Package mcp implements the Model Context Protocol server that exposes
// every operation in Nudgeway's OpenAPI 3.1 specification as an MCP tool.
//
// The package is deliberately provider-free: it only depends on the standard
// library, gopkg.in/yaml.v3, and internal/api/openapi (for the embedded
// spec). It never imports anything under internal/providers/*, keeping the
// dependency rule (CLAUDE.md §4) intact.
package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// httpMethods is the ordered list of HTTP methods the tool generator will
// consider when walking each path item. Order matters for deterministic
// tool listing.
var httpMethods = []string{"get", "put", "post", "delete", "patch", "head", "options"}

// Tool is a single MCP tool descriptor exposed via `tools/list`.
type Tool struct {
	// Name is the tool identifier the MCP client uses in `tools/call`.
	// It equals the OpenAPI operationId.
	Name string `json:"name"`
	// Description is a short human summary shown in the MCP client UI.
	Description string `json:"description"`
	// InputSchema is a JSON Schema object describing the arguments the
	// caller may pass in `tools/call`.
	InputSchema map[string]any `json:"inputSchema"`

	// Method, PathTemplate, PathParams, QueryParams, HasBody, RequestBodyRequired
	// are used by the HTTP forwarder to reconstruct a request from
	// tool-call arguments. They are intentionally unexported from the wire
	// format via json tags on Tool being the only exported view.
	Method              string   `json:"-"`
	PathTemplate        string   `json:"-"`
	PathParams          []string `json:"-"`
	QueryParams         []string `json:"-"`
	HasBody             bool     `json:"-"`
	RequestBodyRequired bool     `json:"-"`
}

// rawSpec is the minimal shape of the OpenAPI document the MCP layer needs.
type rawSpec struct {
	Paths map[string]yaml.Node `yaml:"paths"`
}

type rawParameter struct {
	Name        string    `yaml:"name"`
	In          string    `yaml:"in"`
	Required    bool      `yaml:"required"`
	Description string    `yaml:"description"`
	Schema      yaml.Node `yaml:"schema"`
}

type rawOperation struct {
	OperationID string         `yaml:"operationId"`
	Summary     string         `yaml:"summary"`
	Description string         `yaml:"description"`
	Tags        []string       `yaml:"tags"`
	Parameters  []rawParameter `yaml:"parameters"`
	RequestBody *rawRequestBody `yaml:"requestBody"`
}

type rawRequestBody struct {
	Required bool                       `yaml:"required"`
	Content  map[string]rawMediaType    `yaml:"content"`
}

type rawMediaType struct {
	Schema yaml.Node `yaml:"schema"`
}

type rawPathItem struct {
	Parameters []rawParameter `yaml:"parameters"`
	// method-specific fields are decoded on demand.
}

// LoadTools parses the given OpenAPI YAML bytes and returns one Tool per
// operationId. Operations without an operationId are skipped with a
// warning-shaped error appended to the returned slice's second value.
func LoadTools(specBytes []byte) ([]Tool, error) {
	var top rawSpec
	if err := yaml.Unmarshal(specBytes, &top); err != nil {
		return nil, fmt.Errorf("parse openapi yaml: %w", err)
	}

	var tools []Tool

	pathKeys := make([]string, 0, len(top.Paths))
	for k := range top.Paths {
		pathKeys = append(pathKeys, k)
	}
	sort.Strings(pathKeys)

	for _, path := range pathKeys {
		itemNode := top.Paths[path]

		var pathItem rawPathItem
		if err := itemNode.Decode(&pathItem); err != nil {
			return nil, fmt.Errorf("decode path %q: %w", path, err)
		}

		// Walk each HTTP method key on the path item.
		methodMap := map[string]yaml.Node{}
		if err := itemNode.Decode(&methodMap); err != nil {
			return nil, fmt.Errorf("decode path %q as map: %w", path, err)
		}

		for _, method := range httpMethods {
			opNode, ok := methodMap[method]
			if !ok {
				continue
			}

			var op rawOperation
			if err := opNode.Decode(&op); err != nil {
				return nil, fmt.Errorf("decode operation %s %s: %w", method, path, err)
			}
			if op.OperationID == "" {
				continue
			}

			tool := buildTool(path, method, pathItem.Parameters, op)
			tools = append(tools, tool)
		}
	}

	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// buildTool assembles a single Tool from an operation plus the shared
// path-level parameters.
func buildTool(path, method string, sharedParams []rawParameter, op rawOperation) Tool {
	// Merge shared path-level parameters with op-level. Op-level wins on
	// name collision.
	seen := map[string]bool{}
	var params []rawParameter
	for _, p := range op.Parameters {
		key := p.In + ":" + p.Name
		seen[key] = true
		params = append(params, p)
	}
	for _, p := range sharedParams {
		key := p.In + ":" + p.Name
		if seen[key] {
			continue
		}
		params = append(params, p)
	}

	properties := map[string]any{}
	var required []string
	var pathParams, queryParams []string

	for _, p := range params {
		schema := yamlNodeToJSON(p.Schema)
		if schema == nil {
			schema = map[string]any{"type": "string"}
		}
		if p.Description != "" {
			if m, ok := schema.(map[string]any); ok {
				if _, has := m["description"]; !has {
					m["description"] = p.Description
				}
			}
		}
		properties[p.Name] = schema
		if p.Required || p.In == "path" {
			required = append(required, p.Name)
		}
		switch p.In {
		case "path":
			pathParams = append(pathParams, p.Name)
		case "query":
			queryParams = append(queryParams, p.Name)
		}
	}

	hasBody := false
	bodyRequired := false
	if op.RequestBody != nil {
		hasBody = true
		bodyRequired = op.RequestBody.Required
		// Prefer application/json, fall back to any other content type.
		var bodySchema any
		if mt, ok := op.RequestBody.Content["application/json"]; ok {
			bodySchema = yamlNodeToJSON(mt.Schema)
		} else {
			for _, mt := range op.RequestBody.Content {
				bodySchema = yamlNodeToJSON(mt.Schema)
				break
			}
		}
		if bodySchema == nil {
			bodySchema = map[string]any{"type": "object"}
		}
		if m, ok := bodySchema.(map[string]any); ok {
			if _, has := m["description"]; !has {
				m["description"] = "Request body sent to the endpoint."
			}
		}
		properties["body"] = bodySchema
		if bodyRequired {
			required = append(required, "body")
		}
	}

	desc := strings.TrimSpace(op.Summary)
	if op.Description != "" {
		if desc != "" {
			desc += "\n\n"
		}
		desc += strings.TrimSpace(op.Description)
	}
	desc = fmt.Sprintf("%s %s\n\n%s", strings.ToUpper(method), path, desc)

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}

	sort.Strings(pathParams)
	sort.Strings(queryParams)

	return Tool{
		Name:                op.OperationID,
		Description:         desc,
		InputSchema:         schema,
		Method:              strings.ToUpper(method),
		PathTemplate:        path,
		PathParams:          pathParams,
		QueryParams:         queryParams,
		HasBody:             hasBody,
		RequestBodyRequired: bodyRequired,
	}
}

// yamlNodeToJSON converts a yaml.Node subtree into the plain JSON-friendly
// Go types (map[string]any, []any, string, float64, bool, nil) so the value
// can be re-emitted as JSON Schema without additional YAML-specific
// scaffolding.
func yamlNodeToJSON(n yaml.Node) any {
	if n.IsZero() {
		return nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return yamlNodeToJSON(*n.Content[0])
	case yaml.MappingNode:
		out := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			out[k] = yamlNodeToJSON(*n.Content[i+1])
		}
		return out
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, yamlNodeToJSON(*c))
		}
		return out
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!bool":
			return n.Value == "true"
		case "!!int":
			var i int64
			if _, err := fmt.Sscanf(n.Value, "%d", &i); err == nil {
				return i
			}
			return n.Value
		case "!!float":
			var f float64
			if _, err := fmt.Sscanf(n.Value, "%g", &f); err == nil {
				return f
			}
			return n.Value
		case "!!null":
			return nil
		default:
			return n.Value
		}
	default:
		return nil
	}
}

// ToolsAsJSON marshals a slice of tools to the JSON shape MCP clients
// expect from `tools/list`.
func ToolsAsJSON(tools []Tool) ([]byte, error) {
	b, err := json.MarshalIndent(tools, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tools: %w", err)
	}
	return b, nil
}
