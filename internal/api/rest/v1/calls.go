package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	appaudit "github.com/fullwa/fullwa/internal/application/audit"
	appcall "github.com/fullwa/fullwa/internal/application/call"
	daudit "github.com/fullwa/fullwa/internal/domain/audit"
	calldom "github.com/fullwa/fullwa/internal/domain/call"
	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	dintegration "github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/rbac"
	"github.com/fullwa/fullwa/internal/domain/user"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	"github.com/fullwa/fullwa/internal/ports/calling"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// CallsDeps bundles state needed by the calls REST handler.
type CallsDeps struct {
	// Service is the application-layer entry point. Nil silently omits
	// every /api/v1/calls route.
	Service *appcall.Service
	// Audit records a row per successful permission-request send. Optional —
	// nil disables audit recording for the permission-request endpoint.
	Audit *appaudit.Service
	// Logger receives handler-level warnings.
	Logger *slog.Logger
}

// mountCalls installs the /api/v1/calls routes on mux.
//
// authed is the standard authenticated + CSRF-protected chain (for
// mutations). base is used to install the read-only listings and the
// recording proxy (safe methods; CSRF is a no-op on GET).
func mountCalls(
	mux Registrar,
	base func(http.Handler) http.Handler,
	authed func(http.Handler) http.Handler,
	deps CallsDeps,
) {
	if deps.Service == nil {
		return
	}
	h := &callsHandler{d: deps}

	readGate := func(next http.Handler) http.Handler {
		return base(middleware.RequireAuth(
			middleware.RequirePermission(rbac.PermCallsRead)(next),
		))
	}
	writeGate := func(next http.Handler) http.Handler {
		return authed(middleware.RequirePermission(rbac.PermCallsManage)(next))
	}

	mux.Handle("GET /api/v1/calls", readGate(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/calls/{id}", readGate(http.HandlerFunc(h.get)))
	mux.Handle("GET /api/v1/calls/{id}/recording", readGate(http.HandlerFunc(h.recording)))
	mux.Handle("GET /api/v1/calls/{id}/transcript", readGate(http.HandlerFunc(h.transcript)))
	mux.Handle("GET /api/v1/calls/{id}/session", readGate(http.HandlerFunc(h.session)))
	mux.Handle("POST /api/v1/calls", writeGate(http.HandlerFunc(h.initiate)))
	mux.Handle("POST /api/v1/calls/permission-request", writeGate(http.HandlerFunc(h.permissionRequest)))
	mux.Handle("POST /api/v1/calls/{id}/answer", writeGate(http.HandlerFunc(h.answer)))
	mux.Handle("POST /api/v1/calls/{id}/reject", writeGate(http.HandlerFunc(h.reject)))
	mux.Handle("POST /api/v1/calls/{id}/end", writeGate(http.HandlerFunc(h.end)))
}

// callsHandler bundles state for each endpoint.
type callsHandler struct{ d CallsDeps }

// callDTO is the JSON shape the operator UI consumes.
type callDTO struct {
	ID                 string         `json:"id"`
	OrgID              string         `json:"org_id"`
	IntegrationID      string         `json:"integration_id,omitempty"`
	BusinessEndpointID string         `json:"business_endpoint_id,omitempty"`
	ContactID          string         `json:"contact_id,omitempty"`
	ContactName        string         `json:"contact_name,omitempty"`
	SessionID          string         `json:"session_id,omitempty"`
	ConversationID     string         `json:"conversation_id,omitempty"`
	Provider           string         `json:"provider"`
	ProviderCallID     string         `json:"provider_call_id"`
	Direction          string         `json:"direction"`
	Status             string         `json:"status"`
	// From is the phone number of the caller (E.164). Retained for backwards
	// compatibility; new UI code should prefer Phone + BSUID.
	From string `json:"from,omitempty"`
	// To is the phone number of the callee. Retained for backwards
	// compatibility; new UI code should prefer Phone + BSUID.
	To string `json:"to,omitempty"`
	// Phone is the E.164 phone number of the customer end of the call —
	// From for inbound calls, To for outbound. Duplicated from From/To for
	// clarity in the UI.
	Phone string `json:"phone,omitempty"`
	// BSUID is the customer's business-scoped user id — the primary
	// identity in the UI. From FromUserID for inbound calls, ToUserID for
	// outbound.
	BSUID            string         `json:"bsuid,omitempty"`
	FromUserID       string         `json:"from_user_id,omitempty"`
	ToUserID         string         `json:"to_user_id,omitempty"`
	StartedAt        string         `json:"started_at,omitempty"`
	AnsweredAt       string         `json:"answered_at,omitempty"`
	EndedAt          string         `json:"ended_at,omitempty"`
	DurationSeconds  int            `json:"duration_seconds"`
	HangupReason     string         `json:"hangup_reason,omitempty"`
	RecordingURL     string         `json:"recording_url,omitempty"`
	TranscriptionRef string         `json:"transcription_ref,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at,omitempty"`
}

// callListResponse wraps a page.
type callListResponse struct {
	Items      []callDTO `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// initiateRequest is the POST /api/v1/calls body.
type initiateRequest struct {
	IntegrationID  string                        `json:"integration_id"`
	To             string                        `json:"to"`
	ToUserID       string                        `json:"to_user_id,omitempty"`
	ContactID      string                        `json:"contact_id,omitempty"`
	IdempotencyKey string                        `json:"idempotency_key,omitempty"`
	Recording      *calling.RecordingOptions     `json:"recording,omitempty"`
	Transcription  *calling.TranscriptionOptions `json:"transcription,omitempty"`
}

// initiateResponse is the 202 body.
type initiateResponse struct {
	CallID string `json:"call_id"`
	Status string `json:"status"`
}

// rejectRequest is the POST /api/v1/calls/{id}/reject body.
type rejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

// list handles GET /api/v1/calls.
func (h *callsHandler) list(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	q := r.URL.Query()
	filter := repository.CallListFilter{
		Cursor: q.Get("cursor"),
	}
	if v := q.Get("status"); v != "" {
		filter.Status = calldom.Status(v)
	}
	if v := q.Get("direction"); v != "" {
		filter.Direction = calldom.Direction(v)
	}
	if v := q.Get("contact_id"); v != "" {
		cid := contact.ID(v)
		filter.ContactID = &cid
	}
	if v := q.Get("conversation_id"); v != "" {
		cid := conversation.ID(v)
		filter.ConversationID = &cid
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid since (want RFC3339)")
			return
		}
		filter.Since = t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid until (want RFC3339)")
			return
		}
		filter.Until = t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid limit (1..200)")
			return
		}
		filter.Limit = n
	}
	page, err := h.d.Service.List(r.Context(), organization.ID(pr.OrgID), filter)
	if err != nil {
		h.logger().Warn("calls list failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "calls list failed")
		return
	}
	// Collect the contact ids present on the page so we can batch-resolve
	// display names in one pass. Missing links (calls not yet stitched to
	// a contact) fall through with empty contact_name.
	ids := make([]contact.ID, 0, len(page.Items))
	for _, c := range page.Items {
		if c.ContactID != nil {
			ids = append(ids, *c.ContactID)
		}
	}
	names := h.d.Service.ResolveContactNames(r.Context(), organization.ID(pr.OrgID), ids)
	items := make([]callDTO, 0, len(page.Items))
	for _, c := range page.Items {
		items = append(items, toCallDTOWithContact(c, names))
	}
	writeJSON(w, http.StatusOK, callListResponse{Items: items, NextCursor: page.NextCursor})
}

// get handles GET /api/v1/calls/{id}.
func (h *callsHandler) get(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	c, err := h.d.Service.Get(r.Context(), organization.ID(pr.OrgID), calldom.ID(id))
	if err != nil {
		if errors.Is(err, appcall.ErrCallNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "call not found")
			return
		}
		h.logger().Warn("call get failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("call_id", id),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "call get failed: "+err.Error())
		return
	}
	var names map[contact.ID]string
	if c.ContactID != nil {
		names = h.d.Service.ResolveContactNames(r.Context(), organization.ID(pr.OrgID), []contact.ID{*c.ContactID})
	}
	writeJSON(w, http.StatusOK, toCallDTOWithContact(c, names))
}

// initiate handles POST /api/v1/calls.
func (h *callsHandler) initiate(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var body initiateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	reqID := middleware.RequestIDFrom(r.Context())
	res, err := h.d.Service.RequestCall(r.Context(), appcall.InitiateRequest{
		OrgID:          pr.OrgID,
		IntegrationID:  body.IntegrationID,
		To:             body.To,
		ToUserID:       body.ToUserID,
		ContactID:      body.ContactID,
		IdempotencyKey: body.IdempotencyKey,
		CorrelationID:  reqID,
		Recording:      body.Recording,
		Transcription:  body.Transcription,
	})
	if err != nil {
		var permErr *appcall.PermissionErr
		switch {
		case errors.As(err, &permErr):
			// 428 Precondition Required: the recipient has not granted the
			// business call permission. Surface the status + expiration so
			// the UI can render the correct affordance (send-request CTA).
			status := permErr.Info.Status
			if status == "" {
				status = "no_permission"
			}
			extras := map[string]any{"permission_status": status}
			if permErr.Info.ExpiresAt > 0 {
				extras["expiration_time"] = permErr.Info.ExpiresAt
			}
			writeProblemExtras(w, r, http.StatusPreconditionRequired, "permission_missing",
				fmt.Sprintf("recipient has not granted call permission (status=%q); ask them to accept a permission request first", status),
				extras,
			)
		case errors.Is(err, appcall.ErrCallValidation):
			writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
		case errors.Is(err, appcall.ErrCallIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		case errors.Is(err, calldom.ErrProviderUnsupported):
			writeProblem(w, r, http.StatusNotImplemented, "not_supported", err.Error())
		default:
			h.logger().Warn("call.initiate failed",
				slog.String("request_id", reqID),
				slog.String("org_id", pr.OrgID),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(initiateResponse{CallID: res.CallID, Status: res.Status})
}

// answerRequestFeature is a per-feature (recording / transcription) toggle
// in the /answer request body. Bare `{"enabled":true}` is enough; purpose
// + announcement_language default to sensible values in the adapter.
type answerRequestFeature struct {
	Enabled              bool   `json:"enabled"`
	Purpose              string `json:"purpose,omitempty"`
	AnnouncementLanguage string `json:"announcement_language,omitempty"`
}

// answerRequest is the POST /api/v1/calls/{id}/answer body. All fields
// are optional — an empty body performs a bare accept (legacy behaviour).
type answerRequest struct {
	SDP           string                `json:"sdp,omitempty"`
	Recording     *answerRequestFeature `json:"recording,omitempty"`
	Transcription *answerRequestFeature `json:"transcription,omitempty"`
}

// answer handles POST /api/v1/calls/{id}/answer. Accepts an optional
// browser-produced SDP answer plus recording / transcription selections.
// Bare (empty) body preserves the legacy bare-accept behaviour so older
// callers keep working.
func (h *callsHandler) answer(w http.ResponseWriter, r *http.Request) {
	var body answerRequest
	if r.Body != nil {
		// Ignore decode errors on empty bodies — bare accept is legal.
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	var rec *calling.RecordingOptions
	if body.Recording != nil && body.Recording.Enabled {
		rec = &calling.RecordingOptions{
			Enabled:              true,
			Purpose:              body.Recording.Purpose,
			AnnouncementLanguage: body.Recording.AnnouncementLanguage,
		}
	}
	var tr *calling.TranscriptionOptions
	if body.Transcription != nil && body.Transcription.Enabled {
		tr = &calling.TranscriptionOptions{
			Enabled:              true,
			Purpose:              body.Transcription.Purpose,
			AnnouncementLanguage: body.Transcription.AnnouncementLanguage,
		}
	}
	h.actionOnCall(w, r, func(orgID organization.ID, id calldom.ID) error {
		if body.SDP == "" && rec == nil && tr == nil {
			return h.d.Service.Answer(r.Context(), orgID, id)
		}
		return h.d.Service.AnswerWithSession(r.Context(), orgID, id, body.SDP, rec, tr)
	})
}

// sessionResponse is the GET /api/v1/calls/{id}/session body.
type sessionResponse struct {
	SDPType string `json:"sdp_type"`
	SDP     string `json:"sdp"`
}

// session handles GET /api/v1/calls/{id}/session. Returns the SDP offer
// captured on the provider's `connect` webhook so the operator's browser
// can build a WebRTC answer. 404 when no offer is stored.
func (h *callsHandler) session(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	sdp, sdpType, err := h.d.Service.GetOfferSDP(r.Context(), organization.ID(pr.OrgID), calldom.ID(id))
	if err != nil {
		if errors.Is(err, appcall.ErrCallNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "call not found")
			return
		}
		h.logger().Warn("call session lookup failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("call_id", id),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "call session lookup failed")
		return
	}
	if sdp == "" {
		writeProblem(w, r, http.StatusNotFound, "not_found", "no SDP offer stored for this call")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{SDPType: sdpType, SDP: sdp})
}

// reject handles POST /api/v1/calls/{id}/reject.
func (h *callsHandler) reject(w http.ResponseWriter, r *http.Request) {
	var body rejectRequest
	// The body is optional. Ignore decode errors on empty bodies.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	h.actionOnCall(w, r, func(orgID organization.ID, id calldom.ID) error {
		return h.d.Service.Reject(r.Context(), orgID, id, body.Reason)
	})
}

// end handles POST /api/v1/calls/{id}/end.
func (h *callsHandler) end(w http.ResponseWriter, r *http.Request) {
	h.actionOnCall(w, r, func(orgID organization.ID, id calldom.ID) error {
		return h.d.Service.End(r.Context(), orgID, id)
	})
}

// recording handles GET /api/v1/calls/{id}/recording. Streams the bytes
// through from the provider. This is intentionally a proxy so the browser
// never sees the Meta-issued short-lived URL (which requires a Bearer
// token to fetch).
func (h *callsHandler) recording(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	body, ctype, err := h.d.Service.GetRecording(r.Context(), organization.ID(pr.OrgID), calldom.ID(id))
	if err != nil {
		if errors.Is(err, appcall.ErrCallNotFound) {
			writeProblem(w, r, http.StatusNotFound, "not_found", "call not found")
			return
		}
		if errors.Is(err, appcall.ErrCallIntegrationMissing) {
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
			return
		}
		h.logger().Warn("call recording proxy failed", slog.Any("err", err))
		writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		return
	}
	defer func() { _ = body.Close() }()
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		h.logger().Warn("call recording stream failed", slog.Any("err", err))
	}
}

// transcript handles GET /api/v1/calls/{id}/transcript. Streams the raw
// transcript JSON returned by the provider. When no transcription_ref
// is stamped yet the endpoint returns 409 not_available so the UI can
// render a "not available yet" affordance.
func (h *callsHandler) transcript(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	raw, err := h.d.Service.GetTranscript(r.Context(), organization.ID(pr.OrgID), calldom.ID(id))
	if err != nil {
		switch {
		case errors.Is(err, appcall.ErrCallNotFound):
			writeProblem(w, r, http.StatusNotFound, "not_found", "call not found")
		case errors.Is(err, appcall.ErrTranscriptNotAvailable):
			writeProblem(w, r, http.StatusConflict, "not_available", "transcript not available yet")
		case errors.Is(err, appcall.ErrCallIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		default:
			h.logger().Warn("call transcript proxy failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("call_id", id),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if len(raw) == 0 {
		// Defensive fallback: hand back the ref so the UI still has a
		// signal that a transcript exists but the provider returned an
		// empty body.
		_, _ = w.Write([]byte(`{}`))
		return
	}
	_, _ = w.Write(raw)
}

// permissionRequestBody is the POST /api/v1/calls/permission-request body.
type permissionRequestBody struct {
	IntegrationID string `json:"integration_id"`
	To            string `json:"to"`
	Prompt        string `json:"prompt,omitempty"`
}

// permissionRequestResponse is the 200 body — just the wamid.
type permissionRequestResponse struct {
	WAMID string `json:"wamid"`
}

// permissionRequest handles POST /api/v1/calls/permission-request. Sends
// an interactive call_permission_request message to the recipient so they
// can grant call permission — a prerequisite for outbound calls when the
// current permission is "no_permission".
func (h *callsHandler) permissionRequest(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var body permissionRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.IntegrationID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "integration_id is required")
		return
	}
	if body.To == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "to is required")
		return
	}
	wamid, err := h.d.Service.SendPermissionRequest(
		r.Context(),
		organization.ID(pr.OrgID),
		dintegration.ID(body.IntegrationID),
		body.To,
		body.Prompt,
	)
	if err != nil {
		switch {
		case errors.Is(err, appcall.ErrCallValidation):
			writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
		case errors.Is(err, appcall.ErrCallIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		default:
			h.logger().Warn("call.permission_request failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	h.audit(r, pr, daudit.CallPermissionRequested, "integration", body.IntegrationID, map[string]any{
		"to":    body.To,
		"wamid": wamid,
	})
	writeJSON(w, http.StatusOK, permissionRequestResponse{WAMID: wamid})
}

// audit persists an audit row for a successful mutation. Nil deps.Audit
// silently skips (opt-out for slim deploys); background goroutine so a
// slow audit write never blocks the response.
func (h *callsHandler) audit(
	r *http.Request,
	pr middleware.Principal,
	action daudit.Action,
	resourceType, resourceID string,
	meta map[string]any,
) {
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
	go h.d.Audit.Record(context.Background(), entry)
}

// actionOnCall is the shared shape for answer / reject / end.
func (h *callsHandler) actionOnCall(w http.ResponseWriter, r *http.Request, op func(organization.ID, calldom.ID) error) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "id required")
		return
	}
	if err := op(organization.ID(pr.OrgID), calldom.ID(id)); err != nil {
		switch {
		case errors.Is(err, appcall.ErrCallNotFound):
			writeProblem(w, r, http.StatusNotFound, "not_found", "call not found")
		case errors.Is(err, appcall.ErrCallIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		case errors.Is(err, calldom.ErrProviderUnsupported):
			writeProblem(w, r, http.StatusNotImplemented, "not_supported", err.Error())
		default:
			h.logger().Warn("call action failed", slog.Any("err", err))
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *callsHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// toCallDTO flattens a domain Call into the JSON shape without any
// contact-name resolution.
func toCallDTO(c calldom.Call) callDTO {
	return toCallDTOWithContact(c, nil)
}

// toCallDTOWithContact flattens a domain Call into the JSON shape and
// populates the customer-facing display fields (contact_name, bsuid,
// phone) derived from the call direction. names is the resolved
// display-name map; a nil map is treated the same as a miss.
func toCallDTOWithContact(c calldom.Call, names map[contact.ID]string) callDTO {
	dto := callDTO{
		ID:               string(c.ID),
		OrgID:            string(c.OrgID),
		IntegrationID:    c.IntegrationID,
		Provider:         c.Provider,
		ProviderCallID:   c.ProviderCallID,
		Direction:        string(c.Direction),
		Status:           string(c.Status),
		From:             c.From,
		To:               c.To,
		FromUserID:       c.FromUserID,
		ToUserID:         c.ToUserID,
		DurationSeconds:  c.DurationSeconds,
		HangupReason:     c.HangupReason,
		RecordingURL:     c.RecordingURL,
		TranscriptionRef: c.TranscriptionRef,
		Metadata:         c.Extras,
		CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.BusinessEndpointID != nil {
		dto.BusinessEndpointID = string(*c.BusinessEndpointID)
	}
	if c.ContactID != nil {
		dto.ContactID = string(*c.ContactID)
		if n, ok := names[*c.ContactID]; ok {
			dto.ContactName = n
		}
	}
	// Phone + BSUID pick the customer-side identity based on direction.
	// Inbound: customer is From/FromUserID. Outbound: customer is To/ToUserID.
	if c.Direction == calldom.DirectionOutbound {
		dto.Phone = c.To
		dto.BSUID = c.ToUserID
	} else {
		dto.Phone = c.From
		dto.BSUID = c.FromUserID
	}
	if c.SessionID != nil {
		dto.SessionID = string(*c.SessionID)
	}
	if c.ConversationID != nil {
		dto.ConversationID = string(*c.ConversationID)
	}
	if c.StartedAt != nil {
		dto.StartedAt = c.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if c.AnsweredAt != nil {
		dto.AnsweredAt = c.AnsweredAt.UTC().Format(time.RFC3339Nano)
	}
	if c.EndedAt != nil {
		dto.EndedAt = c.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	if c.UpdatedAt != nil {
		dto.UpdatedAt = c.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	// If the RecordingURL points at Meta rather than our proxy, expose a
	// friendlier proxy URL for the frontend to hit — avoids CORS + hides
	// the short-lived token.
	if dto.RecordingURL != "" {
		dto.RecordingURL = fmt.Sprintf("/api/v1/calls/%s/recording", dto.ID)
	}
	return dto
}
