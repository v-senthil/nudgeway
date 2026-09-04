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

// TemplateSummary is one row of a list response — provider-native fields
// only. The application layer maps this into the canonical
// template.Template.
type TemplateSummary struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Language   string           `json:"language"`
	Status     string           `json:"status"`
	Category   string           `json:"category"`
	Components []map[string]any `json:"components,omitempty"`
	// QualityScore mirrors Meta's per-template quality signal
	// ("green"/"yellow"/"red"/"unknown"). Empty when Meta did not return
	// one on this list call.
	QualityScore string `json:"quality_score,omitempty"`
}

// TemplateCreateRequest is the typed form of TemplateSubmission the
// application layer prefers — same shape, different name so the caller
// site reads as "create a template on the provider".
type TemplateCreateRequest = TemplateSubmission

// TemplateCreateResult is the shape Meta returns from POST message_templates:
// the assigned id + the current review status + the (possibly re-derived)
// category. The application layer stamps these back on the local row.
type TemplateCreateResult struct {
	ID       string `json:"id"`
	Status   string `json:"status,omitempty"`
	Category string `json:"category,omitempty"`
}

// TemplateStatus is the typed form of a get-by-id response. Status is what
// callers usually want; Category is preserved for the sync path to
// reconcile category re-classifications.
type TemplateStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Language string `json:"language,omitempty"`
	Status   string `json:"status,omitempty"`
	Category string `json:"category,omitempty"`
}

// ListTemplatesRaw returns the raw Meta message_templates payload. The
// application layer prefers ListTemplates for typed access; this stays
// exported so operator tooling can dump the pre-mapped body.
func (p *Provider) ListTemplatesRaw(ctx context.Context) (json.RawMessage, error) {
	return p.client.listTemplates(ctx)
}

// ListTemplates returns the parsed WABA template list. The tracer emits
// one execution-log event per request; parsing failures are surfaced as
// errors so a partial page never lands as an empty list.
func (p *Provider) ListTemplates(ctx context.Context) ([]TemplateSummary, error) {
	raw, err := p.client.listTemplates(ctx)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data []TemplateSummary `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("whatsapp: decode list_templates: %w", err)
	}
	return env.Data, nil
}

// CreateTemplateRaw submits a template for Meta review and returns the raw
// response payload.
func (p *Provider) CreateTemplateRaw(ctx context.Context, tmpl TemplateSubmission) (json.RawMessage, error) {
	body, err := json.Marshal(tmpl)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: encode template: %w", err)
	}
	return p.client.createTemplate(ctx, body)
}

// CreateTemplate submits a template for Meta review and returns the
// parsed create result (id + status).
func (p *Provider) CreateTemplate(ctx context.Context, req TemplateCreateRequest) (TemplateCreateResult, error) {
	raw, err := p.CreateTemplateRaw(ctx, req)
	if err != nil {
		return TemplateCreateResult{}, err
	}
	var out TemplateCreateResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return TemplateCreateResult{}, fmt.Errorf("whatsapp: decode create_template: %w", err)
	}
	return out, nil
}

// GetTemplateStatusRaw fetches a single template by id and returns the
// raw payload.
func (p *Provider) GetTemplateStatusRaw(ctx context.Context, templateID string) (json.RawMessage, error) {
	if templateID == "" {
		return nil, fmt.Errorf("whatsapp: GetTemplateStatus: templateID required")
	}
	return p.client.getTemplateStatus(ctx, templateID)
}

// GetTemplateStatus fetches a single template by id and returns the
// parsed status view.
func (p *Provider) GetTemplateStatus(ctx context.Context, templateID string) (TemplateStatus, error) {
	raw, err := p.GetTemplateStatusRaw(ctx, templateID)
	if err != nil {
		return TemplateStatus{}, err
	}
	var out TemplateStatus
	if err := json.Unmarshal(raw, &out); err != nil {
		return TemplateStatus{}, fmt.Errorf("whatsapp: decode get_template_status: %w", err)
	}
	return out, nil
}
