package whatsapp

// This file exposes the Meta WABA-level analytics surface: messaging,
// conversation, pricing, call, and template analytics. Each Meta field
// (`analytics`, `conversation_analytics`, `pricing_analytics`,
// `call_analytics`, `template_analytics`) is requested off the WABA
// endpoint via `GET /{waba-id}?fields=<field>.<filters>` and routed
// through client.doJSON so the tracer records every round trip.
//
// Reference (source of truth, do NOT invent):
//   - ~/Documents/whatsapp_doc_tracker/docs/analytics.md
//
// Response structs mirror the Meta JSON shapes one-to-one. Optional
// fields carry `,omitempty` so the settings drawer round-trips the
// same shape it receives.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---- shared filter helpers ------------------------------------------------

// bracketList encodes a Meta filter list as "[a,b,c]" with each entry
// left un-quoted (Meta's `fields=` grammar rejects quoted enum values in
// most contexts). Empty slices return "[]" which Meta interprets as
// "all".
func bracketList(vals []string) string {
	if len(vals) == 0 {
		return "[]"
	}
	return "[" + strings.Join(vals, ",") + "]"
}

// quotedBracketList is the string-quoted variant Meta requires for
// phone number arrays and country code arrays inside `fields=`.
func quotedBracketList(vals []string) string {
	if len(vals) == 0 {
		return "[]"
	}
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = "\"" + v + "\""
	}
	return "[" + strings.Join(q, ",") + "]"
}

// ---- messaging analytics --------------------------------------------------

// MessagingAnalyticsRequest is the caller-supplied filter set for the
// `analytics` field. Start / End are UNIX seconds; Granularity is one
// of "HALF_HOUR" | "DAY" | "MONTH" per Meta.
type MessagingAnalyticsRequest struct {
	Start        int64    `json:"start"`
	End          int64    `json:"end"`
	Granularity  string   `json:"granularity"`
	PhoneNumbers []string `json:"phone_numbers,omitempty"`
	ProductTypes []int    `json:"product_types,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
}

// MessagingAnalyticsDataPoint is one bucket in the messaging analytics
// series.
type MessagingAnalyticsDataPoint struct {
	Start     int64 `json:"start"`
	End       int64 `json:"end"`
	Sent      int64 `json:"sent"`
	Delivered int64 `json:"delivered"`
}

// MessagingAnalyticsPayload is the inner `analytics` object Meta wraps
// the data points in.
type MessagingAnalyticsPayload struct {
	PhoneNumbers []string                      `json:"phone_numbers,omitempty"`
	CountryCodes []string                      `json:"country_codes,omitempty"`
	Granularity  string                        `json:"granularity,omitempty"`
	DataPoints   []MessagingAnalyticsDataPoint `json:"data_points,omitempty"`
}

// MessagingAnalyticsResponse mirrors Meta's `{analytics, id}` envelope.
type MessagingAnalyticsResponse struct {
	Analytics MessagingAnalyticsPayload `json:"analytics"`
	ID        string                    `json:"id,omitempty"`
}

// MessagingAnalytics fetches the `analytics` field for the WABA. The
// caller-supplied filters are encoded into Meta's dot-suffixed
// `fields=<field>.filter(value)...` grammar; the request is routed
// through client.doJSON so the tracer captures the round trip under
// `meta_messaging_analytics`.
func (p *Provider) MessagingAnalytics(ctx context.Context, wabaID string, req MessagingAnalyticsRequest) (MessagingAnalyticsResponse, error) {
	if wabaID == "" {
		wabaID = p.cfg.WABAID
	}
	if wabaID == "" {
		return MessagingAnalyticsResponse{}, fmt.Errorf("whatsapp: MessagingAnalytics: waba id required")
	}
	if req.Granularity == "" {
		return MessagingAnalyticsResponse{}, fmt.Errorf("whatsapp: MessagingAnalytics: granularity required")
	}

	var b strings.Builder
	b.WriteString("analytics")
	b.WriteString(fmt.Sprintf(".start(%d)", req.Start))
	b.WriteString(fmt.Sprintf(".end(%d)", req.End))
	b.WriteString(fmt.Sprintf(".granularity(%s)", req.Granularity))
	if len(req.PhoneNumbers) > 0 {
		b.WriteString(".phone_numbers(")
		b.WriteString(quotedBracketList(req.PhoneNumbers))
		b.WriteString(")")
	}
	if len(req.ProductTypes) > 0 {
		s := make([]string, len(req.ProductTypes))
		for i, v := range req.ProductTypes {
			s[i] = strconv.Itoa(v)
		}
		b.WriteString(".product_types(")
		b.WriteString(bracketList(s))
		b.WriteString(")")
	}
	if len(req.CountryCodes) > 0 {
		b.WriteString(".country_codes(")
		b.WriteString(quotedBracketList(req.CountryCodes))
		b.WriteString(")")
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s?fields=%s",
		p.cfg.baseURL(), p.cfg.version(), wabaID, url.QueryEscape(b.String()),
	)
	var out MessagingAnalyticsResponse
	if err := p.client.doJSON(ctx, "meta_messaging_analytics", http.MethodGet, endpoint, nil, &out); err != nil {
		return MessagingAnalyticsResponse{}, fmt.Errorf("whatsapp: messaging analytics: %w", err)
	}
	return out, nil
}

// ---- conversation analytics -----------------------------------------------

// ConversationAnalyticsRequest is the caller-supplied filter set for
// the `conversation_analytics` field.
type ConversationAnalyticsRequest struct {
	Start                   int64    `json:"start"`
	End                     int64    `json:"end"`
	Granularity             string   `json:"granularity"`
	PhoneNumbers            []string `json:"phone_numbers,omitempty"`
	MetricTypes             []string `json:"metric_types,omitempty"`
	ConversationCategories  []string `json:"conversation_categories,omitempty"`
	ConversationTypes       []string `json:"conversation_types,omitempty"`
	ConversationDirections  []string `json:"conversation_directions,omitempty"`
	Dimensions              []string `json:"dimensions,omitempty"`
	CountryCodes            []string `json:"country_codes,omitempty"`
}

// ConversationAnalyticsDataPoint mirrors Meta's per-bucket record. All
// optional fields fall out on the read side; a data point may only carry
// the subset selected by `dimensions`.
type ConversationAnalyticsDataPoint struct {
	Start                 int64   `json:"start"`
	End                   int64   `json:"end"`
	Conversation          int64   `json:"conversation,omitempty"`
	PhoneNumber           string  `json:"phone_number,omitempty"`
	Country               string  `json:"country,omitempty"`
	ConversationType      string  `json:"conversation_type,omitempty"`
	ConversationDirection string  `json:"conversation_direction,omitempty"`
	ConversationCategory  string  `json:"conversation_category,omitempty"`
	Cost                  float64 `json:"cost,omitempty"`
}

// ConversationAnalyticsData wraps a series of data points.
type ConversationAnalyticsData struct {
	DataPoints []ConversationAnalyticsDataPoint `json:"data_points,omitempty"`
}

// ConversationAnalyticsPayload mirrors Meta's `{data:[...]}` inner
// envelope.
type ConversationAnalyticsPayload struct {
	Data []ConversationAnalyticsData `json:"data,omitempty"`
}

// ConversationAnalyticsResponse mirrors Meta's full response.
type ConversationAnalyticsResponse struct {
	ConversationAnalytics ConversationAnalyticsPayload `json:"conversation_analytics"`
	ID                    string                       `json:"id,omitempty"`
}

// ConversationAnalytics fetches the `conversation_analytics` field
// for the WABA. Routed through client.doJSON as
// `meta_conversation_analytics`.
func (p *Provider) ConversationAnalytics(ctx context.Context, wabaID string, req ConversationAnalyticsRequest) (ConversationAnalyticsResponse, error) {
	if wabaID == "" {
		wabaID = p.cfg.WABAID
	}
	if wabaID == "" {
		return ConversationAnalyticsResponse{}, fmt.Errorf("whatsapp: ConversationAnalytics: waba id required")
	}
	if req.Granularity == "" {
		return ConversationAnalyticsResponse{}, fmt.Errorf("whatsapp: ConversationAnalytics: granularity required")
	}

	var b strings.Builder
	b.WriteString("conversation_analytics")
	b.WriteString(fmt.Sprintf(".start(%d)", req.Start))
	b.WriteString(fmt.Sprintf(".end(%d)", req.End))
	b.WriteString(fmt.Sprintf(".granularity(%s)", req.Granularity))
	if req.PhoneNumbers != nil {
		b.WriteString(".phone_numbers(")
		b.WriteString(quotedBracketList(req.PhoneNumbers))
		b.WriteString(")")
	}
	if len(req.MetricTypes) > 0 {
		b.WriteString(".metric_types(")
		b.WriteString(bracketList(req.MetricTypes))
		b.WriteString(")")
	}
	if len(req.ConversationCategories) > 0 {
		b.WriteString(".conversation_categories(")
		b.WriteString(quotedBracketList(req.ConversationCategories))
		b.WriteString(")")
	}
	if len(req.ConversationTypes) > 0 {
		b.WriteString(".conversation_types(")
		b.WriteString(bracketList(req.ConversationTypes))
		b.WriteString(")")
	}
	if len(req.ConversationDirections) > 0 {
		b.WriteString(".conversation_directions(")
		b.WriteString(bracketList(req.ConversationDirections))
		b.WriteString(")")
	}
	if len(req.Dimensions) > 0 {
		b.WriteString(".dimensions(")
		b.WriteString(bracketList(req.Dimensions))
		b.WriteString(")")
	}
	if len(req.CountryCodes) > 0 {
		b.WriteString(".country_codes(")
		b.WriteString(quotedBracketList(req.CountryCodes))
		b.WriteString(")")
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s?fields=%s",
		p.cfg.baseURL(), p.cfg.version(), wabaID, url.QueryEscape(b.String()),
	)
	var out ConversationAnalyticsResponse
	if err := p.client.doJSON(ctx, "meta_conversation_analytics", http.MethodGet, endpoint, nil, &out); err != nil {
		return ConversationAnalyticsResponse{}, fmt.Errorf("whatsapp: conversation analytics: %w", err)
	}
	return out, nil
}

// ---- pricing analytics ----------------------------------------------------

// PricingAnalyticsRequest is the caller-supplied filter set for the
// `pricing_analytics` field.
type PricingAnalyticsRequest struct {
	Start              int64    `json:"start"`
	End                int64    `json:"end"`
	Granularity        string   `json:"granularity"`
	PhoneNumbers       []string `json:"phone_numbers,omitempty"`
	CountryCodes       []string `json:"country_codes,omitempty"`
	MetricTypes        []string `json:"metric_types,omitempty"`
	PricingTypes       []string `json:"pricing_types,omitempty"`
	PricingCategories  []string `json:"pricing_categories,omitempty"`
	Dimensions         []string `json:"dimensions,omitempty"`
}

// PricingAnalyticsDataPoint mirrors Meta's per-bucket record. `Tier`
// is omitted for free messages; `Volume` and `Cost` are only populated
// for the metric types selected.
type PricingAnalyticsDataPoint struct {
	Start           int64   `json:"start"`
	End             int64   `json:"end"`
	Country         string  `json:"country,omitempty"`
	PhoneNumber     string  `json:"phone_number,omitempty"`
	Tier            string  `json:"tier,omitempty"`
	PricingType     string  `json:"pricing_type,omitempty"`
	PricingCategory string  `json:"pricing_category,omitempty"`
	Volume          int64   `json:"volume,omitempty"`
	Cost            float64 `json:"cost,omitempty"`
}

// PricingAnalyticsData wraps a series of data points.
type PricingAnalyticsData struct {
	DataPoints []PricingAnalyticsDataPoint `json:"data_points,omitempty"`
}

// PricingAnalyticsPayload mirrors Meta's `{data:[...]}` inner envelope.
type PricingAnalyticsPayload struct {
	Data []PricingAnalyticsData `json:"data,omitempty"`
}

// PricingAnalyticsResponse mirrors Meta's full response.
type PricingAnalyticsResponse struct {
	PricingAnalytics PricingAnalyticsPayload `json:"pricing_analytics"`
	ID               string                  `json:"id,omitempty"`
}

// PricingAnalytics fetches the `pricing_analytics` field for the WABA.
// Routed through client.doJSON as `meta_pricing_analytics`.
func (p *Provider) PricingAnalytics(ctx context.Context, wabaID string, req PricingAnalyticsRequest) (PricingAnalyticsResponse, error) {
	if wabaID == "" {
		wabaID = p.cfg.WABAID
	}
	if wabaID == "" {
		return PricingAnalyticsResponse{}, fmt.Errorf("whatsapp: PricingAnalytics: waba id required")
	}
	if req.Granularity == "" {
		return PricingAnalyticsResponse{}, fmt.Errorf("whatsapp: PricingAnalytics: granularity required")
	}

	var b strings.Builder
	b.WriteString("pricing_analytics")
	b.WriteString(fmt.Sprintf(".start(%d)", req.Start))
	b.WriteString(fmt.Sprintf(".end(%d)", req.End))
	b.WriteString(fmt.Sprintf(".granularity(%s)", req.Granularity))
	if len(req.PhoneNumbers) > 0 {
		b.WriteString(".phone_numbers(")
		b.WriteString(quotedBracketList(req.PhoneNumbers))
		b.WriteString(")")
	}
	if len(req.CountryCodes) > 0 {
		b.WriteString(".country_codes(")
		b.WriteString(quotedBracketList(req.CountryCodes))
		b.WriteString(")")
	}
	if len(req.MetricTypes) > 0 {
		b.WriteString(".metric_types(")
		b.WriteString(bracketList(req.MetricTypes))
		b.WriteString(")")
	}
	if len(req.PricingTypes) > 0 {
		b.WriteString(".pricing_types(")
		b.WriteString(bracketList(req.PricingTypes))
		b.WriteString(")")
	}
	if len(req.PricingCategories) > 0 {
		b.WriteString(".pricing_categories(")
		b.WriteString(bracketList(req.PricingCategories))
		b.WriteString(")")
	}
	if len(req.Dimensions) > 0 {
		b.WriteString(".dimensions(")
		b.WriteString(bracketList(req.Dimensions))
		b.WriteString(")")
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s?fields=%s",
		p.cfg.baseURL(), p.cfg.version(), wabaID, url.QueryEscape(b.String()),
	)
	var out PricingAnalyticsResponse
	if err := p.client.doJSON(ctx, "meta_pricing_analytics", http.MethodGet, endpoint, nil, &out); err != nil {
		return PricingAnalyticsResponse{}, fmt.Errorf("whatsapp: pricing analytics: %w", err)
	}
	return out, nil
}

// ---- call analytics -------------------------------------------------------

// CallAnalyticsMetaRequest is the caller-supplied filter set for the
// `call_analytics` field. Suffixed with `Meta` so it does not collide
// with the existing Nudgeway calling application-service types.
type CallAnalyticsMetaRequest struct {
	Start        int64    `json:"start"`
	End          int64    `json:"end"`
	Granularity  string   `json:"granularity"`
	PhoneNumbers []string `json:"phone_numbers,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
	Directions   []string `json:"directions,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	MetricTypes  []string `json:"metric_types,omitempty"`
}

// CallAnalyticsMetaDataPoint mirrors Meta's per-bucket record.
type CallAnalyticsMetaDataPoint struct {
	Start           int64   `json:"start"`
	End             int64   `json:"end"`
	Count           int64   `json:"count,omitempty"`
	Cost            float64 `json:"cost,omitempty"`
	AverageDuration int64   `json:"average_duration,omitempty"`
	PhoneNumber     string  `json:"phone_number,omitempty"`
	Country         string  `json:"country,omitempty"`
	Direction       string  `json:"direction,omitempty"`
}

// CallAnalyticsMetaPayload mirrors Meta's `call_analytics` inner
// object. Meta returns granularity + optional flatten of the direction
// filter alongside the bucket list.
type CallAnalyticsMetaPayload struct {
	Granularity string                       `json:"granularity,omitempty"`
	Directions  json.RawMessage              `json:"directions,omitempty"`
	DataPoints  []CallAnalyticsMetaDataPoint `json:"data_points,omitempty"`
}

// CallAnalyticsMetaResponse mirrors Meta's full response.
type CallAnalyticsMetaResponse struct {
	CallAnalytics CallAnalyticsMetaPayload `json:"call_analytics"`
	ID            string                   `json:"id,omitempty"`
}

// CallAnalyticsMeta fetches the `call_analytics` field for the WABA.
// Routed through client.doJSON as `meta_call_analytics`.
func (p *Provider) CallAnalyticsMeta(ctx context.Context, wabaID string, req CallAnalyticsMetaRequest) (CallAnalyticsMetaResponse, error) {
	if wabaID == "" {
		wabaID = p.cfg.WABAID
	}
	if wabaID == "" {
		return CallAnalyticsMetaResponse{}, fmt.Errorf("whatsapp: CallAnalyticsMeta: waba id required")
	}
	if req.Granularity == "" {
		return CallAnalyticsMetaResponse{}, fmt.Errorf("whatsapp: CallAnalyticsMeta: granularity required")
	}

	var b strings.Builder
	b.WriteString("call_analytics")
	b.WriteString(fmt.Sprintf(".start(%d)", req.Start))
	b.WriteString(fmt.Sprintf(".end(%d)", req.End))
	b.WriteString(fmt.Sprintf(".granularity(%s)", req.Granularity))
	if len(req.PhoneNumbers) > 0 {
		b.WriteString(".phone_numbers(")
		b.WriteString(quotedBracketList(req.PhoneNumbers))
		b.WriteString(")")
	}
	if len(req.CountryCodes) > 0 {
		b.WriteString(".country_codes(")
		b.WriteString(quotedBracketList(req.CountryCodes))
		b.WriteString(")")
	}
	if len(req.Directions) > 0 {
		b.WriteString(".directions(")
		b.WriteString(bracketList(req.Directions))
		b.WriteString(")")
	}
	if len(req.Dimensions) > 0 {
		b.WriteString(".dimensions(")
		b.WriteString(bracketList(req.Dimensions))
		b.WriteString(")")
	}
	if len(req.MetricTypes) > 0 {
		b.WriteString(".metric_types(")
		b.WriteString(bracketList(req.MetricTypes))
		b.WriteString(")")
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s?fields=%s",
		p.cfg.baseURL(), p.cfg.version(), wabaID, url.QueryEscape(b.String()),
	)
	var out CallAnalyticsMetaResponse
	if err := p.client.doJSON(ctx, "meta_call_analytics", http.MethodGet, endpoint, nil, &out); err != nil {
		return CallAnalyticsMetaResponse{}, fmt.Errorf("whatsapp: call analytics: %w", err)
	}
	return out, nil
}

// ---- template analytics ---------------------------------------------------

// TemplateAnalyticsRequest is the caller-supplied filter set for the
// `template_analytics` edge. Meta serves this via
// `GET /{waba-id}/template_analytics` rather than the `?fields=` grammar
// used by the other analytics surfaces, but the shape below mirrors what
// callers actually pass. The 90-day lookback is enforced by Meta.
type TemplateAnalyticsRequest struct {
	Start           int64    `json:"start"`
	End             int64    `json:"end"`
	Granularity     string   `json:"granularity"`
	TemplateIDs     []string `json:"template_ids"`
	MetricTypes     []string `json:"metric_types,omitempty"`
	ProductType     string   `json:"product_type,omitempty"`
	UseWABATimezone bool     `json:"use_waba_timezone,omitempty"`
}

// TemplateAnalyticsCost is one entry in a data point's cost array
// (`amount_spent`, `cost_per_delivered`, `cost_per_url_button_click`).
type TemplateAnalyticsCost struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

// TemplateAnalyticsClick is one entry in a data point's clicked array
// (`url_button`, `unique_url_button`, `quick_reply_button`).
type TemplateAnalyticsClick struct {
	Type          string `json:"type"`
	ButtonContent string `json:"button_content,omitempty"`
	Count         int64  `json:"count"`
}

// TemplateAnalyticsDataPoint mirrors Meta's per-template bucket.
type TemplateAnalyticsDataPoint struct {
	TemplateID string                   `json:"template_id"`
	Start      int64                    `json:"start"`
	End        int64                    `json:"end"`
	Sent       int64                    `json:"sent,omitempty"`
	Delivered  int64                    `json:"delivered,omitempty"`
	Read       int64                    `json:"read,omitempty"`
	Clicked    []TemplateAnalyticsClick `json:"clicked,omitempty"`
	Cost       []TemplateAnalyticsCost  `json:"cost,omitempty"`
}

// TemplateAnalyticsBucket is one WABA-timezone bucket in the top-level
// data list.
type TemplateAnalyticsBucket struct {
	WABATimezone string                       `json:"waba_timezone,omitempty"`
	Granularity  string                       `json:"granularity,omitempty"`
	ProductType  string                       `json:"product_type,omitempty"`
	DataPoints   []TemplateAnalyticsDataPoint `json:"data_points,omitempty"`
}

// TemplateAnalyticsPaging mirrors Meta's cursor pagination envelope.
type TemplateAnalyticsPaging struct {
	Cursors struct {
		Before string `json:"before,omitempty"`
		After  string `json:"after,omitempty"`
	} `json:"cursors,omitempty"`
}

// TemplateAnalyticsResponse mirrors Meta's full response.
type TemplateAnalyticsResponse struct {
	Data   []TemplateAnalyticsBucket `json:"data"`
	Paging *TemplateAnalyticsPaging  `json:"paging,omitempty"`
}

// TemplateAnalytics fetches the `template_analytics` edge for the WABA.
// Unlike the other analytics fields, Meta serves this via a dedicated
// sub-resource with conventional query-string filters. Routed through
// client.doJSON as `meta_template_analytics`.
func (p *Provider) TemplateAnalytics(ctx context.Context, wabaID string, req TemplateAnalyticsRequest) (TemplateAnalyticsResponse, error) {
	if wabaID == "" {
		wabaID = p.cfg.WABAID
	}
	if wabaID == "" {
		return TemplateAnalyticsResponse{}, fmt.Errorf("whatsapp: TemplateAnalytics: waba id required")
	}
	if req.Granularity == "" {
		req.Granularity = "DAILY"
	}
	if len(req.TemplateIDs) == 0 {
		return TemplateAnalyticsResponse{}, fmt.Errorf("whatsapp: TemplateAnalytics: template_ids required")
	}

	q := url.Values{}
	q.Set("start", strconv.FormatInt(req.Start, 10))
	q.Set("end", strconv.FormatInt(req.End, 10))
	q.Set("granularity", req.Granularity)
	// Meta expects a bare bracketed list for template_ids (no percent
	// encoding around the brackets themselves would be ideal, but url.Values
	// takes care of the whole value — Meta accepts the encoded form).
	q.Set("template_ids", "["+strings.Join(req.TemplateIDs, ",")+"]")
	if len(req.MetricTypes) > 0 {
		q.Set("metric_types", strings.Join(req.MetricTypes, ","))
	}
	if req.ProductType != "" {
		q.Set("product_type", req.ProductType)
	}
	if req.UseWABATimezone {
		q.Set("use_waba_timezone", "true")
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s/template_analytics?%s",
		p.cfg.baseURL(), p.cfg.version(), wabaID, q.Encode(),
	)
	var out TemplateAnalyticsResponse
	if err := p.client.doJSON(ctx, "meta_template_analytics", http.MethodGet, endpoint, nil, &out); err != nil {
		return TemplateAnalyticsResponse{}, fmt.Errorf("whatsapp: template analytics: %w", err)
	}
	return out, nil
}
