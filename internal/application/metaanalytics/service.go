// Package metaanalytics is the application-layer entry point for the
// Meta WhatsApp Business Account (WABA) analytics surface. It mirrors
// the five Meta analytics fields — messaging, conversation, pricing,
// call, template — as one method each and delegates to a
// provider-agnostic Resolver so provider SDKs never leak into the
// application layer.
//
// Reference (source of truth, do NOT invent):
//   - ~/Documents/whatsapp_doc_tracker/docs/analytics.md
//
// Design notes:
//   - The Deps / Resolver pattern mirrors
//     internal/application/integrationsettings so the wiring in
//     cmd/server is symmetric.
//   - Request / response DTOs mirror Meta's JSON one-to-one so REST
//     handlers can round-trip the payload without a second translation
//     layer. The provider-neutral names avoid the "Meta" prefix on
//     types the service exposes.
package metaanalytics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ErrNotFound is returned when the requested integration does not exist
// (or belongs to a different organization). REST layer maps to 404.
var ErrNotFound = errors.New("integration not found")

// ---- request DTOs ---------------------------------------------------------

// MessagingAnalyticsRequest mirrors Meta's `analytics` filter set.
type MessagingAnalyticsRequest struct {
	Start        int64    `json:"start"`
	End          int64    `json:"end"`
	Granularity  string   `json:"granularity"`
	PhoneNumbers []string `json:"phone_numbers,omitempty"`
	ProductTypes []int    `json:"product_types,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
}

// ConversationAnalyticsRequest mirrors Meta's `conversation_analytics`
// filter set.
type ConversationAnalyticsRequest struct {
	Start                  int64    `json:"start"`
	End                    int64    `json:"end"`
	Granularity            string   `json:"granularity"`
	PhoneNumbers           []string `json:"phone_numbers,omitempty"`
	MetricTypes            []string `json:"metric_types,omitempty"`
	ConversationCategories []string `json:"conversation_categories,omitempty"`
	ConversationTypes      []string `json:"conversation_types,omitempty"`
	ConversationDirections []string `json:"conversation_directions,omitempty"`
	Dimensions             []string `json:"dimensions,omitempty"`
	CountryCodes           []string `json:"country_codes,omitempty"`
}

// PricingAnalyticsRequest mirrors Meta's `pricing_analytics` filter set.
type PricingAnalyticsRequest struct {
	Start             int64    `json:"start"`
	End               int64    `json:"end"`
	Granularity       string   `json:"granularity"`
	PhoneNumbers      []string `json:"phone_numbers,omitempty"`
	CountryCodes      []string `json:"country_codes,omitempty"`
	MetricTypes       []string `json:"metric_types,omitempty"`
	PricingTypes      []string `json:"pricing_types,omitempty"`
	PricingCategories []string `json:"pricing_categories,omitempty"`
	Dimensions        []string `json:"dimensions,omitempty"`
}

// CallAnalyticsRequest mirrors Meta's `call_analytics` filter set.
type CallAnalyticsRequest struct {
	Start        int64    `json:"start"`
	End          int64    `json:"end"`
	Granularity  string   `json:"granularity"`
	PhoneNumbers []string `json:"phone_numbers,omitempty"`
	CountryCodes []string `json:"country_codes,omitempty"`
	Directions   []string `json:"directions,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	MetricTypes  []string `json:"metric_types,omitempty"`
}

// TemplateAnalyticsRequest mirrors Meta's `template_analytics` filter
// set. `TemplateIDs` is required; Meta caps at 10 ids per request.
type TemplateAnalyticsRequest struct {
	Start           int64    `json:"start"`
	End             int64    `json:"end"`
	Granularity     string   `json:"granularity"`
	TemplateIDs     []string `json:"template_ids"`
	MetricTypes     []string `json:"metric_types,omitempty"`
	ProductType     string   `json:"product_type,omitempty"`
	UseWABATimezone bool     `json:"use_waba_timezone,omitempty"`
}

// ---- response DTOs --------------------------------------------------------
//
// Response shapes mirror Meta's JSON one-to-one. REST handlers stream
// the DTOs straight through, so any change to Meta's shape is a
// documentable API break.

// MessagingAnalyticsDataPoint is one bucket in the messaging series.
type MessagingAnalyticsDataPoint struct {
	Start     int64 `json:"start"`
	End       int64 `json:"end"`
	Sent      int64 `json:"sent"`
	Delivered int64 `json:"delivered"`
}

// MessagingAnalyticsPayload mirrors Meta's inner `analytics` object.
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

// ConversationAnalyticsDataPoint mirrors Meta's per-bucket record.
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

// PricingAnalyticsDataPoint mirrors Meta's per-bucket record.
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

// CallAnalyticsDataPoint mirrors Meta's per-bucket record.
type CallAnalyticsDataPoint struct {
	Start           int64   `json:"start"`
	End             int64   `json:"end"`
	Count           int64   `json:"count,omitempty"`
	Cost            float64 `json:"cost,omitempty"`
	AverageDuration int64   `json:"average_duration,omitempty"`
	PhoneNumber     string  `json:"phone_number,omitempty"`
	Country         string  `json:"country,omitempty"`
	Direction       string  `json:"direction,omitempty"`
}

// CallAnalyticsPayload mirrors Meta's inner `call_analytics` object.
type CallAnalyticsPayload struct {
	Granularity string                   `json:"granularity,omitempty"`
	DataPoints  []CallAnalyticsDataPoint `json:"data_points,omitempty"`
}

// CallAnalyticsResponse mirrors Meta's full response.
type CallAnalyticsResponse struct {
	CallAnalytics CallAnalyticsPayload `json:"call_analytics"`
	ID            string               `json:"id,omitempty"`
}

// TemplateAnalyticsCost is one entry in a data point's cost array.
type TemplateAnalyticsCost struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

// TemplateAnalyticsClick is one entry in a data point's clicked array.
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

// ---- ports ----------------------------------------------------------------

// MetaAnalyticsProvider is the provider-agnostic surface each adapter
// must implement to be usable from this service. Implementations live
// in cmd/server so the application layer never imports concrete
// provider packages. `wabaID` may be empty — the adapter is expected
// to fall back to its stored WABA id when so.
type MetaAnalyticsProvider interface {
	MessagingAnalytics(ctx context.Context, wabaID string, req MessagingAnalyticsRequest) (MessagingAnalyticsResponse, error)
	ConversationAnalytics(ctx context.Context, wabaID string, req ConversationAnalyticsRequest) (ConversationAnalyticsResponse, error)
	PricingAnalytics(ctx context.Context, wabaID string, req PricingAnalyticsRequest) (PricingAnalyticsResponse, error)
	CallAnalytics(ctx context.Context, wabaID string, req CallAnalyticsRequest) (CallAnalyticsResponse, error)
	TemplateAnalytics(ctx context.Context, wabaID string, req TemplateAnalyticsRequest) (TemplateAnalyticsResponse, error)
}

// Resolver builds a MetaAnalyticsProvider for a given integration +
// secrets combination. Implemented by cmd/server so provider imports
// never leak into internal/application.
type Resolver interface {
	MetaAnalytics(ctx context.Context, providerKey string, integ dintegration.Integration, secrets map[string]string) (MetaAnalyticsProvider, error)
}

// IntegrationSecretsRepo is the read path that returns an integration
// together with its decrypted secrets. Mirrors the shape used by
// internal/application/integrationsettings so wire-up can reuse the
// same repository implementation.
type IntegrationSecretsRepo interface {
	repository.IntegrationRepo
	GetWithSecrets(ctx context.Context, orgID organization.ID, id dintegration.ID) (dintegration.Integration, map[string]string, error)
}

// ---- service --------------------------------------------------------------

// Service is the use-case entry point for the Meta analytics endpoints.
type Service struct {
	integrations IntegrationSecretsRepo
	providers    Resolver
	logger       *slog.Logger
}

// Deps bundles the constructor arguments of Service.
type Deps struct {
	// Integrations exposes the row + decrypted secrets (required).
	Integrations IntegrationSecretsRepo
	// Providers resolves a MetaAnalyticsProvider for the integration
	// (required).
	Providers Resolver
	// Logger receives handler-level warnings; nil falls back to
	// slog.Default.
	Logger *slog.Logger
}

// NewService constructs a Service. Panics if required deps are nil.
func NewService(d Deps) *Service {
	if d.Integrations == nil {
		panic("metaanalytics.NewService: Integrations required")
	}
	if d.Providers == nil {
		panic("metaanalytics.NewService: Providers required")
	}
	return &Service{
		integrations: d.Integrations,
		providers:    d.Providers,
		logger:       d.Logger,
	}
}

// resolve loads the integration + decrypted secrets and hands them to
// the configured Resolver. Reserved secret keys mirror the send /
// settings paths so the provider tracer can tag every emitted
// execution-log row.
func (s *Service) resolve(ctx context.Context, orgID organization.ID, id dintegration.ID) (MetaAnalyticsProvider, string, error) {
	row, secrets, err := s.integrations.GetWithSecrets(ctx, orgID, id)
	if err != nil {
		return nil, "", fmt.Errorf("metaanalytics: load integration: %w", err)
	}
	if row.ID == "" {
		return nil, "", ErrNotFound
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	wabaID, _ := row.Config["waba_id"].(string)
	if pn, ok := row.Config["phone_number_id"].(string); ok {
		secrets["phone_number_id"] = pn
	}
	if wabaID != "" {
		secrets["waba_id"] = wabaID
	}
	secrets["_integration_id"] = string(row.ID)
	secrets["_org_id"] = string(row.OrgID)
	prov, err := s.providers.MetaAnalytics(ctx, row.Provider, row, secrets)
	if err != nil {
		return nil, "", fmt.Errorf("metaanalytics: resolve provider: %w", err)
	}
	return prov, wabaID, nil
}

// MessagingAnalytics is the passthrough for Meta's `analytics` field.
func (s *Service) MessagingAnalytics(ctx context.Context, orgID organization.ID, id dintegration.ID, req MessagingAnalyticsRequest) (MessagingAnalyticsResponse, error) {
	pc, wabaID, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return MessagingAnalyticsResponse{}, err
	}
	return pc.MessagingAnalytics(ctx, wabaID, req)
}

// ConversationAnalytics is the passthrough for Meta's
// `conversation_analytics` field.
func (s *Service) ConversationAnalytics(ctx context.Context, orgID organization.ID, id dintegration.ID, req ConversationAnalyticsRequest) (ConversationAnalyticsResponse, error) {
	pc, wabaID, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return ConversationAnalyticsResponse{}, err
	}
	return pc.ConversationAnalytics(ctx, wabaID, req)
}

// PricingAnalytics is the passthrough for Meta's `pricing_analytics`
// field.
func (s *Service) PricingAnalytics(ctx context.Context, orgID organization.ID, id dintegration.ID, req PricingAnalyticsRequest) (PricingAnalyticsResponse, error) {
	pc, wabaID, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return PricingAnalyticsResponse{}, err
	}
	return pc.PricingAnalytics(ctx, wabaID, req)
}

// CallAnalytics is the passthrough for Meta's `call_analytics` field.
func (s *Service) CallAnalytics(ctx context.Context, orgID organization.ID, id dintegration.ID, req CallAnalyticsRequest) (CallAnalyticsResponse, error) {
	pc, wabaID, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return CallAnalyticsResponse{}, err
	}
	return pc.CallAnalytics(ctx, wabaID, req)
}

// TemplateAnalytics is the passthrough for Meta's `template_analytics`
// edge. Note the 90-day lookback + enable_template_analytics
// prerequisite are enforced by Meta, not by this service.
func (s *Service) TemplateAnalytics(ctx context.Context, orgID organization.ID, id dintegration.ID, req TemplateAnalyticsRequest) (TemplateAnalyticsResponse, error) {
	pc, wabaID, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return TemplateAnalyticsResponse{}, err
	}
	return pc.TemplateAnalytics(ctx, wabaID, req)
}
