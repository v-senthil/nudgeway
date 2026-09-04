package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"

	appaudit "github.com/fullwa/fullwa/internal/application/audit"
	appsettings "github.com/fullwa/fullwa/internal/application/integrationsettings"
	daudit "github.com/fullwa/fullwa/internal/domain/audit"
	dintegration "github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/rbac"
	"github.com/fullwa/fullwa/internal/domain/user"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
)

// IntegrationSettingsDeps bundles the settings-drawer endpoints'
// dependencies.
type IntegrationSettingsDeps struct {
	// Service is the application-layer entry point. Nil silently omits
	// every route below so slim deploys can skip the surface.
	Service *appsettings.Service
	// Audit records a row per successful settings mutation. Optional —
	// nil disables audit recording for this endpoint set.
	Audit *appaudit.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// integrationSettingsHandler bundles the state each endpoint func needs.
type integrationSettingsHandler struct{ d IntegrationSettingsDeps }

// mountIntegrationSettings installs the /api/v1/integrations/{id}/...
// settings routes on mux. All routes require auth + integrations.manage;
// state-changing routes also require a valid CSRF double-submit cookie.
func mountIntegrationSettings(mux Registrar, base func(http.Handler) http.Handler, authed func(http.Handler) http.Handler, deps IntegrationSettingsDeps) {
	if deps.Service == nil {
		return
	}
	h := &integrationSettingsHandler{d: deps}
	// GETs: auth + permission, no CSRF (safe method).
	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermIntegrationsManage)(next),
		))
	}
	// Writes: auth + CSRF + permission.
	writeGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermIntegrationsManage)(next))
	}

	mux.Handle("GET /api/v1/integrations/{id}/business-profile", readGate(http.HandlerFunc(h.getBusinessProfile)))
	mux.Handle("PUT /api/v1/integrations/{id}/business-profile", writeGate(http.HandlerFunc(h.putBusinessProfile)))

	mux.Handle("GET /api/v1/integrations/{id}/call-settings", readGate(http.HandlerFunc(h.getCallSettings)))
	mux.Handle("PUT /api/v1/integrations/{id}/call-settings", writeGate(http.HandlerFunc(h.putCallSettings)))

	// The permission-check endpoint gates on calls.read rather than
	// integrations.manage — it's consumed by the "New call" affordance,
	// not the settings drawer proper.
	callsReadGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermCallsRead)(next),
		))
	}
	mux.Handle("GET /api/v1/integrations/{id}/call-permission", callsReadGate(http.HandlerFunc(h.getCallPermission)))

	mux.Handle("GET /api/v1/integrations/{id}/oba-status", readGate(http.HandlerFunc(h.getOBAStatus)))
	mux.Handle("POST /api/v1/integrations/{id}/oba-status/apply", writeGate(http.HandlerFunc(h.applyOBA)))
	mux.Handle("POST /api/v1/integrations/{id}/oba-status/withdraw", writeGate(http.HandlerFunc(h.withdrawOBA)))

	mux.Handle("GET /api/v1/integrations/{id}/username", readGate(http.HandlerFunc(h.getUsername)))
	mux.Handle("PUT /api/v1/integrations/{id}/username", writeGate(http.HandlerFunc(h.putUsername)))
	mux.Handle("DELETE /api/v1/integrations/{id}/username", writeGate(http.HandlerFunc(h.deleteUsername)))
	mux.Handle("GET /api/v1/integrations/{id}/username/suggestions", readGate(http.HandlerFunc(h.getUsernameSuggestions)))

	mux.Handle("GET /api/v1/integrations/{id}/phone-number", readGate(http.HandlerFunc(h.getPhoneNumber)))
}

// pathID extracts and validates the {id} path segment.
func (h *integrationSettingsHandler) pathID(w http.ResponseWriter, r *http.Request) (dintegration.ID, bool) {
	id := dintegration.ID(r.PathValue("id"))
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return "", false
	}
	return id, true
}

// getBusinessProfile handles GET /business-profile.
func (h *integrationSettingsHandler) getBusinessProfile(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	bp, err := h.d.Service.GetBusinessProfile(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_business_profile", err)
		return
	}
	writeJSON(w, http.StatusOK, bp)
}

// putBusinessProfile handles PUT /business-profile.
func (h *integrationSettingsHandler) putBusinessProfile(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	var bp appsettings.BusinessProfileDTO
	if err := json.NewDecoder(r.Body).Decode(&bp); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := h.d.Service.UpdateBusinessProfile(r.Context(), organization.ID(pr.OrgID), id, bp); err != nil {
		h.writeErr(w, r, "update_business_profile", err)
		return
	}
	h.audit(r, pr, daudit.BusinessProfileUpdated, "integration", string(id), map[string]any{
		"about":       bp.About,
		"vertical":    bp.Vertical,
		"has_address": bp.Address != "",
		"websites":    len(bp.Websites),
	})
	// Refetch so the drawer can render the reconciled state without a
	// separate round-trip.
	out, err := h.d.Service.GetBusinessProfile(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_business_profile", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// getCallSettings handles GET /call-settings.
func (h *integrationSettingsHandler) getCallSettings(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	cs, err := h.d.Service.GetCallSettings(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_call_settings", err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

// putCallSettings handles PUT /call-settings.
func (h *integrationSettingsHandler) putCallSettings(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	var cs appsettings.CallSettingsDTO
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := h.d.Service.UpdateCallSettings(r.Context(), organization.ID(pr.OrgID), id, cs); err != nil {
		h.writeErr(w, r, "update_call_settings", err)
		return
	}
	callHoursEnabled := cs.CallHours != nil && cs.CallHours.Status == "ENABLED"
	callHoursRows := 0
	if cs.CallHours != nil {
		callHoursRows = len(cs.CallHours.WeeklyOperatingHours)
	}
	h.audit(r, pr, daudit.CallSettingsUpdated, "integration", string(id), map[string]any{
		"status":                     cs.Status,
		"call_icon_visibility":       cs.CallIconVisibility,
		"callback_permission_status": cs.CallbackPermissionStatus,
		"call_hours_enabled":         callHoursEnabled,
		"call_hours_row_count":       callHoursRows,
	})
	out, err := h.d.Service.GetCallSettings(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_call_settings", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// getCallPermission handles GET /call-permission?to=<E164> — returns the
// current WhatsApp user-call-permission for the recipient, used by the
// "New call" affordance to render a permanent/temporary/no-permission chip
// and enable/disable the Call button.
func (h *integrationSettingsHandler) getCallPermission(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	to := r.URL.Query().Get("to")
	if to == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "to is required")
		return
	}
	pm, err := h.d.Service.GetCallPermission(r.Context(), organization.ID(pr.OrgID), id, to)
	if err != nil {
		if errors.Is(err, appsettings.ErrPermissionUnsupported) {
			writeProblem(w, r, http.StatusNotImplemented, "not_supported",
				"call permission lookup is not wired for this deployment")
			return
		}
		h.writeErr(w, r, "get_call_permission", err)
		return
	}
	writeJSON(w, http.StatusOK, pm)
}

// getOBAStatus handles GET /oba-status.
func (h *integrationSettingsHandler) getOBAStatus(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	out, err := h.d.Service.GetOBAStatus(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_oba_status", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// applyOBA handles POST /oba-status/apply.
func (h *integrationSettingsHandler) applyOBA(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	out, err := h.d.Service.ApplyOBA(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "apply_oba", err)
		return
	}
	h.audit(r, pr, daudit.OBAApplied, "integration", string(id), map[string]any{
		"result_status": out.OBAStatus,
	})
	writeJSON(w, http.StatusOK, out)
}

// withdrawOBA handles POST /oba-status/withdraw.
func (h *integrationSettingsHandler) withdrawOBA(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	out, err := h.d.Service.WithdrawOBA(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "withdraw_oba", err)
		return
	}
	h.audit(r, pr, daudit.OBAWithdrawn, "integration", string(id), map[string]any{
		"result_status": out.OBAStatus,
	})
	writeJSON(w, http.StatusOK, out)
}

// audit persists an audit trail row for the given settings mutation. It is
// fire-and-forget by design — a failed audit write must never break the
// caller's response. Nil deps.Audit silently skips (opt-out for slim
// deploys). The record is enriched with actor + IP + request_id from the
// live HTTP request; the metadata bag is caller-supplied and should
// contain only non-secret diagnostic fields (never tokens or PII).
func (h *integrationSettingsHandler) audit(r *http.Request, pr middleware.Principal, action daudit.Action, resourceType, resourceID string, meta map[string]any) {
	if h.d.Audit == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["request_id"] = middleware.RequestIDFrom(r.Context())
	entry := daudit.Entry{
		OrgID:        organization.ID(pr.OrgID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     meta,
		IP:           net.ParseIP(clientIP(r)),
	}
	if pr.UserID != "" {
		uid := user.ID(pr.UserID)
		entry.ActorUserID = &uid
	}
	// Fire-and-forget from the caller's perspective (Record swallows
	// persistence errors). Use a detached background context so a cancelled
	// request doesn't lose the audit row.
	go h.d.Audit.Record(context.Background(), entry)
}

// getUsername handles GET /username.
func (h *integrationSettingsHandler) getUsername(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	u, err := h.d.Service.GetUsername(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_username", err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// putUsername handles PUT /username — adopt or change the business
// username. The body carries {username, transfer_action?}; transfer_action
// defaults to "none" when omitted.
func (h *integrationSettingsHandler) putUsername(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Username       string `json:"username"`
		TransferAction string `json:"transfer_action,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Username == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "username is required")
		return
	}
	out, err := h.d.Service.SetUsername(r.Context(), organization.ID(pr.OrgID), id, body.Username, body.TransferAction)
	if err != nil {
		h.writeErr(w, r, "set_username", err)
		return
	}
	h.audit(r, pr, daudit.UsernameUpdated, "integration", string(id), map[string]any{
		"username":        body.Username,
		"transfer_action": body.TransferAction,
	})
	writeJSON(w, http.StatusOK, out)
}

// deleteUsername handles DELETE /username — releases the current username.
func (h *integrationSettingsHandler) deleteUsername(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	if err := h.d.Service.DeleteUsername(r.Context(), organization.ID(pr.OrgID), id); err != nil {
		h.writeErr(w, r, "delete_username", err)
		return
	}
	h.audit(r, pr, daudit.UsernameDeleted, "integration", string(id), map[string]any{
		"deleted": true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// getUsernameSuggestions handles GET /username/suggestions.
func (h *integrationSettingsHandler) getUsernameSuggestions(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	sugs, err := h.d.Service.GetUsernameSuggestions(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_username_suggestions", err)
		return
	}
	if sugs == nil {
		sugs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": sugs})
}

// getPhoneNumber handles GET /phone-number — returns the Meta phone-number
// record for the integration's configured phone number id. Empty struct
// with 200 is a valid response when the id is not (yet) part of the WABA.
func (h *integrationSettingsHandler) getPhoneNumber(w http.ResponseWriter, r *http.Request) {
	pr, _ := middleware.PrincipalFrom(r.Context())
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	pn, err := h.d.Service.GetPhoneNumber(r.Context(), organization.ID(pr.OrgID), id)
	if err != nil {
		h.writeErr(w, r, "get_phone_number", err)
		return
	}
	writeJSON(w, http.StatusOK, pn)
}

// writeErr maps an application-layer error to a problem+json body.
// Not-found becomes 404; everything else routes through
// writeProviderProblem so Meta's error code + trace id surface in the
// UI when present.
func (h *integrationSettingsHandler) writeErr(w http.ResponseWriter, r *http.Request, op string, err error) {
	if errors.Is(err, appsettings.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "not_found", "integration not found")
		return
	}
	if h.d.Logger != nil {
		h.d.Logger.Warn("integration settings handler error",
			slog.String("op", op),
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.Any("err", err),
		)
	}
	writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
}
