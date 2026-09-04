package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appmetaanalytics "github.com/v-senthil/nudgeway/internal/application/metaanalytics"
	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/http/middleware"
)

// MetaAnalyticsDeps bundles the Meta analytics endpoints' dependencies.
type MetaAnalyticsDeps struct {
	// Service is the application-layer entry point. Nil silently
	// omits every route below so slim deploys can skip the surface.
	Service *appmetaanalytics.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// metaAnalyticsHandler bundles state for the meta-analytics endpoints.
type metaAnalyticsHandler struct{ d MetaAnalyticsDeps }

// mountMetaAnalytics installs GET
// /api/v1/integrations/{id}/meta-analytics/{surface} on mux. All
// routes are auth-gated + require the `analytics.read` permission.
// Silently omitted when deps.Service is nil.
func mountMetaAnalytics(mux Registrar, authedGET func(http.Handler) http.Handler, deps MetaAnalyticsDeps) {
	if deps.Service == nil {
		return
	}
	h := &metaAnalyticsHandler{d: deps}
	gate := func(next http.Handler) http.Handler {
		return authedGET(middleware.RequirePermission(rbac.PermAnalyticsRead)(next))
	}
	mux.Handle("GET /api/v1/integrations/{id}/meta-analytics/messaging", gate(http.HandlerFunc(h.messaging)))
	mux.Handle("GET /api/v1/integrations/{id}/meta-analytics/conversations", gate(http.HandlerFunc(h.conversations)))
	mux.Handle("GET /api/v1/integrations/{id}/meta-analytics/pricing", gate(http.HandlerFunc(h.pricing)))
	mux.Handle("GET /api/v1/integrations/{id}/meta-analytics/calls", gate(http.HandlerFunc(h.calls)))
	mux.Handle("GET /api/v1/integrations/{id}/meta-analytics/templates", gate(http.HandlerFunc(h.templates)))
}

// pathIntegrationID extracts and validates the {id} path segment.
func (h *metaAnalyticsHandler) pathIntegrationID(w http.ResponseWriter, r *http.Request) (dintegration.ID, bool) {
	id := dintegration.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return "", false
	}
	return id, true
}

// parseTimeRange parses `since` / `until` query params into UNIX
// seconds. Accepts either full RFC 3339 timestamps or bare YYYY-MM-DD
// dates (interpreted as UTC midnight); the latter is what the analytics
// UI's date pickers naturally emit. Both params are required.
func (h *metaAnalyticsHandler) parseTimeRange(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	sinceRaw := r.URL.Query().Get("since")
	untilRaw := r.URL.Query().Get("until")
	if sinceRaw == "" || untilRaw == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "since and until are required (YYYY-MM-DD or RFC3339)")
		return 0, 0, false
	}
	since, ok := parseFlexibleDate(sinceRaw, false)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "validation", "since must be YYYY-MM-DD or RFC3339")
		return 0, 0, false
	}
	until, ok := parseFlexibleDate(untilRaw, true)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "validation", "until must be YYYY-MM-DD or RFC3339")
		return 0, 0, false
	}
	if !until.After(since) {
		writeProblem(w, r, http.StatusBadRequest, "validation", "until must be after since")
		return 0, 0, false
	}
	return since.Unix(), until.Unix(), true
}

// parseFlexibleDate accepts full RFC 3339 (`2026-09-05T12:34:56Z`) or
// bare `YYYY-MM-DD`. Bare dates go UTC midnight; when `endOfDay` is
// true they resolve to 23:59:59 so an inclusive `since=X&until=X`
// still spans that day.
func parseFlexibleDate(raw string, endOfDay bool) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Second), true
		}
		return t, true
	}
	return time.Time{}, false
}

// granularity reads the `granularity` query param with a sensible
// default (DAILY). Meta's messaging analytics accepts `DAY` in place
// of `DAILY`; the caller supplies whichever their surface expects.
func (h *metaAnalyticsHandler) granularity(r *http.Request, def string) string {
	g := strings.TrimSpace(r.URL.Query().Get("granularity"))
	if g == "" {
		return def
	}
	return strings.ToUpper(g)
}

// csvParam splits a comma-separated query param into a trimmed slice.
// Empty string returns nil.
func csvParam(r *http.Request, name string) []string {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// csvIntParam is csvParam for `product_types`.
func csvIntParam(r *http.Request, name string) []int {
	vals := csvParam(r, name)
	if len(vals) == 0 {
		return nil
	}
	out := make([]int, 0, len(vals))
	for _, v := range vals {
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// writeErr maps a service error to a problem+json body. Missing
// integration becomes 404; every other error is treated as a provider
// bad-gateway so the Meta error surfaces to operators.
func (h *metaAnalyticsHandler) writeErr(w http.ResponseWriter, r *http.Request, op string, err error) {
	if errors.Is(err, appmetaanalytics.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "not_found", "integration not found")
		return
	}
	if h.d.Logger != nil {
		h.d.Logger.Warn("meta analytics handler error",
			slog.String("op", op),
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.Any("err", err),
		)
	}
	writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
}

// messaging handles GET /meta-analytics/messaging.
func (h *metaAnalyticsHandler) messaging(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathIntegrationID(w, r)
	if !ok {
		return
	}
	since, until, ok := h.parseTimeRange(w, r)
	if !ok {
		return
	}
	req := appmetaanalytics.MessagingAnalyticsRequest{
		Start:        since,
		End:          until,
		Granularity:  h.granularity(r, "DAY"),
		PhoneNumbers: csvParam(r, "phone_numbers"),
		ProductTypes: csvIntParam(r, "product_types"),
		CountryCodes: csvParam(r, "country_codes"),
	}
	out, err := h.d.Service.MessagingAnalytics(r.Context(), organization.ID(pr.OrgID), id, req)
	if err != nil {
		h.writeErr(w, r, "meta_messaging_analytics", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// conversations handles GET /meta-analytics/conversations.
func (h *metaAnalyticsHandler) conversations(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathIntegrationID(w, r)
	if !ok {
		return
	}
	since, until, ok := h.parseTimeRange(w, r)
	if !ok {
		return
	}
	req := appmetaanalytics.ConversationAnalyticsRequest{
		Start:                  since,
		End:                    until,
		Granularity:            h.granularity(r, "DAILY"),
		PhoneNumbers:           csvParam(r, "phone_numbers"),
		MetricTypes:            csvParam(r, "metric_types"),
		ConversationCategories: csvParam(r, "conversation_categories"),
		ConversationTypes:      csvParam(r, "conversation_types"),
		ConversationDirections: csvParam(r, "conversation_directions"),
		Dimensions:             csvParam(r, "dimensions"),
		CountryCodes:           csvParam(r, "country_codes"),
	}
	out, err := h.d.Service.ConversationAnalytics(r.Context(), organization.ID(pr.OrgID), id, req)
	if err != nil {
		h.writeErr(w, r, "meta_conversation_analytics", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// pricing handles GET /meta-analytics/pricing.
func (h *metaAnalyticsHandler) pricing(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathIntegrationID(w, r)
	if !ok {
		return
	}
	since, until, ok := h.parseTimeRange(w, r)
	if !ok {
		return
	}
	req := appmetaanalytics.PricingAnalyticsRequest{
		Start:             since,
		End:               until,
		Granularity:       h.granularity(r, "DAILY"),
		PhoneNumbers:      csvParam(r, "phone_numbers"),
		CountryCodes:      csvParam(r, "country_codes"),
		MetricTypes:       csvParam(r, "metric_types"),
		PricingTypes:      csvParam(r, "pricing_types"),
		PricingCategories: csvParam(r, "pricing_categories"),
		Dimensions:        csvParam(r, "dimensions"),
	}
	out, err := h.d.Service.PricingAnalytics(r.Context(), organization.ID(pr.OrgID), id, req)
	if err != nil {
		h.writeErr(w, r, "meta_pricing_analytics", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// calls handles GET /meta-analytics/calls.
func (h *metaAnalyticsHandler) calls(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathIntegrationID(w, r)
	if !ok {
		return
	}
	since, until, ok := h.parseTimeRange(w, r)
	if !ok {
		return
	}
	req := appmetaanalytics.CallAnalyticsRequest{
		Start:        since,
		End:          until,
		Granularity:  h.granularity(r, "DAILY"),
		PhoneNumbers: csvParam(r, "phone_numbers"),
		CountryCodes: csvParam(r, "country_codes"),
		Directions:   csvParam(r, "directions"),
		Dimensions:   csvParam(r, "dimensions"),
		MetricTypes:  csvParam(r, "metric_types"),
	}
	out, err := h.d.Service.CallAnalytics(r.Context(), organization.ID(pr.OrgID), id, req)
	if err != nil {
		h.writeErr(w, r, "meta_call_analytics", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// templates handles GET /meta-analytics/templates.
func (h *metaAnalyticsHandler) templates(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathIntegrationID(w, r)
	if !ok {
		return
	}
	since, until, ok := h.parseTimeRange(w, r)
	if !ok {
		return
	}
	templateIDs := csvParam(r, "template_ids")
	if len(templateIDs) == 0 {
		writeProblem(w, r, http.StatusBadRequest, "validation", "template_ids is required (comma-separated)")
		return
	}
	useWABATz := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("use_waba_timezone")), "true")
	req := appmetaanalytics.TemplateAnalyticsRequest{
		Start:           since,
		End:             until,
		Granularity:     h.granularity(r, "DAILY"),
		TemplateIDs:     templateIDs,
		MetricTypes:     csvParam(r, "metric_types"),
		ProductType:     strings.TrimSpace(r.URL.Query().Get("product_type")),
		UseWABATimezone: useWABATz,
	}
	out, err := h.d.Service.TemplateAnalytics(r.Context(), organization.ID(pr.OrgID), id, req)
	if err != nil {
		h.writeErr(w, r, "meta_template_analytics", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
