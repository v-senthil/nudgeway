package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
)

// TemplateSubmission is the minimal set of fields accepted by Meta's
// message_templates create endpoint. Callers pass Components as an opaque
// slice — Meta's component schema is wide and rev's frequently, so we keep
// it flexible rather than modelling every button/header/media variant here.
type TemplateSubmission struct {
	Name       string           `json:"name"`
	Language   string           `json:"language"`
	Category   string           `json:"category"`
	Components []map[string]any `json:"components"`
	// AllowCategoryChange lets Meta re-categorise if warranted.
	AllowCategoryChange bool `json:"allow_category_change,omitempty"`
}

// TemplateInfo is a partial view of a template returned by list/get calls —
// the full payload is preserved as Raw for callers that need it.
type TemplateInfo struct {
	ID       string          `json:"id"`
	Name     string          `json:"name,omitempty"`
	Language string          `json:"language,omitempty"`
	Status   string          `json:"status,omitempty"`
	Category string          `json:"category,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// ListTemplates returns registered templates on the configured WABA.
func (p *Provider) ListTemplates(ctx context.Context) (json.RawMessage, error) {
	return p.client.listTemplates(ctx)
}

// CreateTemplate submits a template for Meta review.
func (p *Provider) CreateTemplate(ctx context.Context, tmpl TemplateSubmission) (json.RawMessage, error) {
	body, err := json.Marshal(tmpl)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: encode template: %w", err)
	}
	return p.client.createTemplate(ctx, body)
}

// GetTemplateStatus fetches a single template by ID.
func (p *Provider) GetTemplateStatus(ctx context.Context, templateID string) (json.RawMessage, error) {
	if templateID == "" {
		return nil, fmt.Errorf("whatsapp: GetTemplateStatus: templateID required")
	}
	return p.client.getTemplateStatus(ctx, templateID)
}
