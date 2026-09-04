package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Forwarder turns an MCP tool-call payload into an HTTP request against the
// Nudgeway REST API and returns the response body plus status information.
type Forwarder struct {
	// BaseURL is the origin of the running Nudgeway server (e.g.
	// http://127.0.0.1:8080). It should not include a trailing slash.
	BaseURL string
	// SessionCookie is the value of the `nudgeway_session` cookie captured
	// from the browser after login.
	SessionCookie string
	// CSRFToken is the CSRF double-submit token. When non-empty it is sent
	// as both an `X-CSRF-Token` header and a `nudgeway_csrf` cookie on
	// state-changing methods (POST, PUT, PATCH, DELETE).
	CSRFToken string
	// HTTPClient overrides the default client. Optional.
	HTTPClient *http.Client
}

// stateChangingMethods lists HTTP verbs that require CSRF protection.
var stateChangingMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// ForwardResult is the structured outcome of a single forwarded call.
type ForwardResult struct {
	// StatusCode is the HTTP status returned by the upstream Nudgeway
	// server (0 if the request never reached the server).
	StatusCode int
	// ContentType is the value of the response Content-Type header.
	ContentType string
	// Body is the raw response body bytes.
	Body []byte
}

// Forward materialises the tool's HTTP request from the given call
// arguments, sends it to the upstream Nudgeway server, and returns the
// result. The caller is responsible for rendering the ForwardResult into
// an MCP tool-call reply.
func (f *Forwarder) Forward(ctx context.Context, tool Tool, args map[string]any) (*ForwardResult, error) {
	if f.BaseURL == "" {
		return nil, fmt.Errorf("forwarder: BaseURL is empty")
	}

	// Substitute path parameters.
	renderedPath := tool.PathTemplate
	for _, name := range tool.PathParams {
		raw, ok := args[name]
		if !ok {
			return nil, fmt.Errorf("forwarder: missing required path parameter %q for tool %q", name, tool.Name)
		}
		renderedPath = strings.ReplaceAll(renderedPath, "{"+name+"}", url.PathEscape(fmt.Sprint(raw)))
	}

	fullURL := strings.TrimRight(f.BaseURL, "/") + renderedPath

	// Attach query parameters.
	if len(tool.QueryParams) > 0 {
		q := url.Values{}
		for _, name := range tool.QueryParams {
			raw, ok := args[name]
			if !ok {
				continue
			}
			switch v := raw.(type) {
			case []any:
				for _, item := range v {
					q.Add(name, fmt.Sprint(item))
				}
			default:
				q.Set(name, fmt.Sprint(v))
			}
		}
		if encoded := q.Encode(); encoded != "" {
			if strings.Contains(fullURL, "?") {
				fullURL += "&" + encoded
			} else {
				fullURL += "?" + encoded
			}
		}
	}

	// Build the request body if declared by the operation.
	var bodyReader io.Reader
	contentType := ""
	if tool.HasBody {
		if raw, ok := args["body"]; ok {
			buf, err := json.Marshal(raw)
			if err != nil {
				return nil, fmt.Errorf("forwarder: marshal body: %w", err)
			}
			bodyReader = bytes.NewReader(buf)
			contentType = "application/json"
		} else if tool.RequestBodyRequired {
			return nil, fmt.Errorf("forwarder: tool %q requires a `body` argument", tool.Name)
		}
	}

	req, err := http.NewRequestWithContext(ctx, tool.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("forwarder: build request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, application/problem+json, text/plain")
	req.Header.Set("User-Agent", "nudgeway-mcp/0.1")

	// Attach the session cookie if provided.
	if f.SessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "nudgeway_session", Value: f.SessionCookie})
	}
	// Attach CSRF for state-changing methods.
	if f.CSRFToken != "" && stateChangingMethods[tool.Method] {
		req.Header.Set("X-CSRF-Token", f.CSRFToken)
		req.AddCookie(&http.Cookie{Name: "nudgeway_csrf", Value: f.CSRFToken})
	}

	client := f.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forwarder: http do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // cap at 4 MiB
	if err != nil {
		return nil, fmt.Errorf("forwarder: read body: %w", err)
	}

	return &ForwardResult{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// RenderText produces the MCP text-content string reported to the client
// for a given ForwardResult. Non-2xx responses are prefixed with the HTTP
// status so the caller sees the failure verbatim.
func RenderText(r *ForwardResult) string {
	if r == nil {
		return ""
	}
	prefix := fmt.Sprintf("HTTP %d %s\n", r.StatusCode, http.StatusText(r.StatusCode))
	body := string(r.Body)
	if body == "" {
		body = "(empty body)"
	}
	return prefix + body
}
