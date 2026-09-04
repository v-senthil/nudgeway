package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	appmsg "github.com/fullwa/fullwa/internal/application/message"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	msgdom "github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// ConversationSummary is the compact conversation row returned by the
// inbox listing. Wire-up time supplies a lister that returns these.
type ConversationSummary struct {
	ID                 string  `json:"id"`
	OrgID              string  `json:"org_id"`
	ContactID          string  `json:"contact_id,omitempty"`
	ContactName        string  `json:"contact_name,omitempty"`
	Type               string  `json:"type"`
	GroupID            string  `json:"group_id,omitempty"`
	Subject            string  `json:"subject,omitempty"`
	Status             string  `json:"status"`
	Channel            string  `json:"channel,omitempty"`
	LastMessageAt      *string `json:"last_message_at,omitempty"`
	LastMessagePreview string  `json:"last_message_preview,omitempty"`
	UnreadCount        int     `json:"unread_count,omitempty"`
	CreatedAt          string  `json:"created_at"`
}

// ConversationsLister returns the inbox summary rows for one org.
type ConversationsLister interface {
	ListConversations(ctx context.Context, orgID string) ([]ConversationSummary, error)
}

// MessagesDeps bundles the dependencies of the message + conversation REST
// endpoints. Provided by cmd/server at wire-up time.
type MessagesDeps struct {
	// Send is the outbound send use-case. Nil disables POST /messages.
	Send *appmsg.SendService
	// Read is the mark-as-read use-case. Nil disables the two
	// POST /messages/{id}/read and POST /conversations/{id}/read routes.
	Read *appmsg.ReadService
	// Messages powers GET /conversations/{id}/messages listings.
	Messages repository.MessageRepo
	// Conversations powers GET /api/v1/conversations. Nil falls back to
	// the empty-list stub when IncludeConversationsIndex is set.
	Conversations ConversationsLister
	// IncludeConversationsIndex installs the /api/v1/conversations handler.
	IncludeConversationsIndex bool
	// Logger receives one structured record per request.
	Logger *slog.Logger
}

// SendMessageRequest is the JSON body of POST /api/v1/messages.
//
// The Type field selects one of the canonical message shapes; the caller
// supplies the matching payload sub-object (text / template / media /
// interactive / location / reaction). The handler serialises the sub-object
// as JSON and hands it to the application service; the provider adapter
// interprets it — nothing provider-specific is inspected in this file.
type SendMessageRequest struct {
	ConversationID string           `json:"conversation_id"`
	Type           string           `json:"type"`
	Text           *json.RawMessage `json:"text,omitempty"`
	Template       *json.RawMessage `json:"template,omitempty"`
	Media          *json.RawMessage `json:"media,omitempty"`
	Interactive    *json.RawMessage `json:"interactive,omitempty"`
	Location       *json.RawMessage `json:"location,omitempty"`
	Reaction       *json.RawMessage `json:"reaction,omitempty"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
}

// SendMessageAccepted is the 202 body of POST /api/v1/messages.
type SendMessageAccepted struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// MessageDTO is the JSON representation of a persisted message row.
type MessageDTO struct {
	ID                       string           `json:"id"`
	ConversationID           string           `json:"conversation_id"`
	SessionID                string           `json:"session_id"`
	ContactID                string           `json:"contact_id"`
	Channel                  string           `json:"channel"`
	Provider                 string           `json:"provider"`
	Direction                string           `json:"direction"`
	Type                     string           `json:"type"`
	Status                   string           `json:"status"`
	Sender                   string           `json:"sender"`
	Recipient                string           `json:"recipient"`
	ProviderMessageID        string           `json:"provider_message_id,omitempty"`
	Text                     string           `json:"text,omitempty"`
	MediaURL                 string           `json:"media_url,omitempty"`
	ContentType              string           `json:"content_type,omitempty"`
	Location                 map[string]any   `json:"location,omitempty"`
	Contacts                 []map[string]any `json:"contacts,omitempty"`
	Reaction                 map[string]any   `json:"reaction,omitempty"`
	Interactive              map[string]any   `json:"interactive,omitempty"`
	Template                 map[string]any   `json:"template,omitempty"`
	ReplyToProviderMessageID string           `json:"reply_to_provider_message_id,omitempty"`
	CreatedAt                string           `json:"created_at"`
	SentAt                   *string          `json:"sent_at,omitempty"`
	DeliveredAt              *string          `json:"delivered_at,omitempty"`
	ReadAt                   *string          `json:"read_at,omitempty"`
}

// MessageListResponse is the 200 body of GET /api/v1/conversations/{id}/messages.
type MessageListResponse struct {
	Items      []MessageDTO `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

// ConversationListResponse is the 200 body of GET /api/v1/conversations.
// A stub while the full list impl is owned by Phase 1 Task 4 peer.
type ConversationListResponse struct {
	Conversations []any  `json:"conversations"`
	NextCursor    string `json:"next_cursor,omitempty"`
}

// mountMessages installs the /api/v1/messages + /api/v1/conversations routes
// on mux using the supplied middleware chain builders.
//
// authed wraps a handler with the standard authenticated + CSRF-protected
// chain; authedGET wraps with the authenticated chain only (CSRF is a no-op
// on safe methods but we still gate on a valid session).
func mountMessages(
	mux Registrar,
	authed func(http.Handler) http.Handler,
	authedGET func(http.Handler) http.Handler,
	deps MessagesDeps,
	includeConversationsList bool,
) {
	if deps.Send == nil && deps.Messages == nil {
		return
	}
	h := &messagesHandler{d: deps}
	if deps.Send != nil {
		mux.Handle("POST /api/v1/messages", authed(http.HandlerFunc(h.send)))
	}
	if deps.Messages != nil {
		mux.Handle("GET /api/v1/conversations/{id}/messages", authedGET(http.HandlerFunc(h.listByConversation)))
	}
	if deps.Read != nil {
		mux.Handle("POST /api/v1/messages/{id}/read", authed(http.HandlerFunc(h.markMessageRead)))
		mux.Handle("POST /api/v1/conversations/{id}/read", authed(http.HandlerFunc(h.markConversationRead)))
	}
	// Placeholder empty-list conversation index so the frontend does not
	// 404 while Phase 1 Task 4 lands the real implementation.
	if includeConversationsList {
		mux.Handle("GET /api/v1/conversations", authedGET(http.HandlerFunc(h.listConversationsStub)))
	}
}

// messagesHandler bundles state for the message + conversation endpoints.
type messagesHandler struct{ d MessagesDeps }

// send handles POST /api/v1/messages.
func (h *messagesHandler) send(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.ConversationID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "conversation_id is required")
		return
	}
	if req.Type == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation", "type is required")
		return
	}
	payload, err := selectPayload(req)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
		return
	}
	reqID := middleware.RequestIDFrom(r.Context())
	res, err := h.d.Send.RequestSend(r.Context(), appmsg.SendRequest{
		OrgID:          pr.OrgID,
		ActorUserID:    pr.UserID,
		ConversationID: req.ConversationID,
		Type:           req.Type,
		Payload:        payload,
		IdempotencyKey: req.IdempotencyKey,
		RequestID:      reqID,
		CorrelationID:  reqID,
	})
	if err != nil {
		switch {
		case errors.Is(err, appmsg.ErrConversationNotFound):
			writeProblem(w, r, http.StatusNotFound, "conversation_not_found", err.Error())
		case errors.Is(err, appmsg.ErrEndpointNotFound),
			errors.Is(err, appmsg.ErrSendIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		case errors.Is(err, appmsg.ErrInvalidPayload):
			writeProblem(w, r, http.StatusBadRequest, "validation", err.Error())
		default:
			h.logger().Warn("message.send request failed",
				slog.String("request_id", reqID),
				slog.String("org_id", pr.OrgID),
				slog.Any("err", err),
			)
			writeProblem(w, r, http.StatusInternalServerError, "internal", "send failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(SendMessageAccepted{
		MessageID: res.MessageID,
		Status:    res.Status,
	})
}

// listByConversation handles GET /api/v1/conversations/{id}/messages.
func (h *messagesHandler) listByConversation(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "conversation id required")
		return
	}
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	page, err := h.d.Messages.ListByConversation(
		r.Context(),
		organization.ID(pr.OrgID),
		conversation.ID(convID),
		repository.MessageListFilter{
			Cursor: r.URL.Query().Get("cursor"),
			Limit:  limit,
		},
	)
	if err != nil {
		h.logger().Warn("message list failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("conversation_id", convID),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "list failed")
		return
	}
	dto := MessageListResponse{
		Items:      make([]MessageDTO, 0, len(page.Messages)),
		NextCursor: page.NextCursor,
	}
	for _, m := range page.Messages {
		dto.Items = append(dto.Items, toMessageDTO(m))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// markMessageRead handles POST /api/v1/messages/{id}/read. It resolves the
// message, calls the channel adapter's MarkAsRead (Meta's blue-tick), and
// stamps read_at locally. Idempotent — a second call is a no-op.
func (h *messagesHandler) markMessageRead(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "message id required")
		return
	}
	err := h.d.Read.MarkRead(r.Context(), organization.ID(pr.OrgID), msgdom.ID(id))
	if err != nil {
		switch {
		case errors.Is(err, appmsg.ErrMessageNotFound):
			writeProblem(w, r, http.StatusNotFound, "message_not_found", err.Error())
		case errors.Is(err, appmsg.ErrReadIntegrationMissing):
			writeProblem(w, r, http.StatusFailedDependency, "integration_missing", err.Error())
		default:
			h.logger().Warn("message.mark_read failed",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("message_id", id),
				slog.Any("err", err),
			)
			writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markConversationRead handles POST /api/v1/conversations/{id}/read. Marks
// the newest inbound-with-wamid unread messages in the conversation as
// read, capped at 50 per call.
func (h *messagesHandler) markConversationRead(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "conversation id required")
		return
	}
	_, err := h.d.Read.MarkConversationRead(r.Context(), organization.ID(pr.OrgID), conversation.ID(convID), 50)
	if err != nil {
		// Batch: log + surface as 502 without leaking internals. Partial
		// successes are already committed; the caller can retry.
		h.logger().Warn("conversation.mark_read partial failure",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("conversation_id", convID),
			slog.Any("err", err),
		)
		writeProviderProblem(w, r, http.StatusBadGateway, "provider_error", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listConversationsStub calls into MessagesDeps.Conversations when provided
// and falls back to an empty list otherwise. Response shape uses `items` to
// match the frontend's ListResponse convention.
func (h *messagesHandler) listConversationsStub(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	items := []ConversationSummary{}
	if h.d.Conversations != nil {
		rows, err := h.d.Conversations.ListConversations(r.Context(), pr.OrgID)
		if err != nil {
			h.logger().Error("list conversations", slog.Any("err", err), slog.String("org_id", pr.OrgID))
			writeProblem(w, r, http.StatusInternalServerError, "internal", "list conversations")
			return
		}
		items = rows
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (h *messagesHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// selectPayload picks the matching payload sub-object off the SendMessageRequest
// based on Type. Returns the raw JSON bytes to hand to the application layer.
//
// Ergonomic shorthand: for type=text the frontend may send `text: "Hello"`
// (a JSON string) instead of the canonical `text: {"body": "Hello"}` shape.
// We normalise here so the composer stays simple.
func selectPayload(req SendMessageRequest) ([]byte, error) {
	switch msgdom.Type(req.Type) {
	case msgdom.TypeText:
		if req.Text == nil {
			return nil, errors.New("text payload required for type=text")
		}
		raw := *req.Text
		// If the operator sent a bare string, wrap it into {"body": "..."}.
		if len(raw) > 0 && raw[0] == '"' {
			var body string
			if err := json.Unmarshal(raw, &body); err != nil {
				return nil, fmt.Errorf("text: %w", err)
			}
			wrapped, err := json.Marshal(map[string]string{"body": body})
			if err != nil {
				return nil, fmt.Errorf("text wrap: %w", err)
			}
			return wrapped, nil
		}
		return raw, nil
	case msgdom.TypeTemplate:
		if req.Template == nil {
			return nil, errors.New("template payload required for type=template")
		}
		return *req.Template, nil
	case msgdom.TypeImage, msgdom.TypeVideo, msgdom.TypeAudio,
		msgdom.TypeDocument, msgdom.TypeSticker:
		if req.Media == nil {
			return nil, errors.New("media payload required for media type")
		}
		return *req.Media, nil
	case msgdom.TypeInteractive:
		if req.Interactive == nil {
			return nil, errors.New("interactive payload required for type=interactive")
		}
		return *req.Interactive, nil
	case msgdom.TypeLocation:
		if req.Location == nil {
			return nil, errors.New("location payload required for type=location")
		}
		return *req.Location, nil
	case msgdom.TypeReaction:
		if req.Reaction == nil {
			return nil, errors.New("reaction payload required for type=reaction")
		}
		return *req.Reaction, nil
	default:
		return nil, errors.New("unsupported message type: " + req.Type)
	}
}

// toMessageDTO flattens a domain Message into the JSON DTO shape.
func toMessageDTO(m msgdom.Message) MessageDTO {
	dto := MessageDTO{
		ID:                string(m.ID),
		ConversationID:    string(m.ConversationID),
		SessionID:         string(m.SessionID),
		ContactID:         string(m.ContactID),
		Channel:           m.Channel,
		Provider:          m.Provider,
		Direction:         string(m.Direction),
		Type:              string(m.MessageType),
		Status:            string(m.Status),
		Sender:            m.SenderIdentity,
		Recipient:         m.RecipientIdentity,
		ProviderMessageID: m.ProviderMessageID,
		CreatedAt:         m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.SentAt != nil {
		s := m.SentAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		dto.SentAt = &s
	}
	if m.DeliveredAt != nil {
		s := m.DeliveredAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		dto.DeliveredAt = &s
	}
	if m.ReadAt != nil {
		s := m.ReadAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		dto.ReadAt = &s
	}
	if m.Metadata != nil {
		if v, ok := m.Metadata["text"].(string); ok {
			dto.Text = v
		}
		// Prefer the self-hosted attachment URL when the inbound pipeline
		// downloaded the media into our own store. This is served through
		// GET /api/v1/media/{key} which is auth-gated + long-lived, so
		// browsers can safely cache it. When we didn't download (older
		// rows, download failure), fall back to the provider-native
		// (short-lived) URL surfaced by the parser.
		if k, ok := m.Metadata["attachment_key"].(string); ok && k != "" {
			dto.MediaURL = "/api/v1/media/" + k
		} else if v, ok := m.Metadata["media_url"].(string); ok {
			dto.MediaURL = v
		}
		if v, ok := m.Metadata["content_type"].(string); ok {
			dto.ContentType = v
		}
		if v, ok := m.Metadata["location"].(map[string]any); ok {
			dto.Location = v
		}
		if v, ok := m.Metadata["contacts"].([]map[string]any); ok {
			dto.Contacts = v
		} else if raw, ok := m.Metadata["contacts"].([]any); ok {
			cards := make([]map[string]any, 0, len(raw))
			for _, r := range raw {
				if mm, ok := r.(map[string]any); ok {
					cards = append(cards, mm)
				}
			}
			if len(cards) > 0 {
				dto.Contacts = cards
			}
		}
		if v, ok := m.Metadata["reaction"].(map[string]any); ok {
			dto.Reaction = v
		}
		if v, ok := m.Metadata["interactive"].(map[string]any); ok {
			dto.Interactive = v
		}
		if v, ok := m.Metadata["template"].(map[string]any); ok {
			dto.Template = v
		}
		if v, ok := m.Metadata["reply_to_wamid"].(string); ok {
			dto.ReplyToProviderMessageID = v
		}
	}
	return dto
}
