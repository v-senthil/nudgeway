package v1

import (
	"encoding/json"
	"errors"
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

// MessagesDeps bundles the dependencies of the message + conversation REST
// endpoints. Provided by cmd/server at wire-up time.
type MessagesDeps struct {
	// Send is the outbound send use-case. Nil disables POST /messages.
	Send *appmsg.SendService
	// Messages powers GET /conversations/{id}/messages listings.
	Messages repository.MessageRepo
	// IncludeConversationsIndex installs a minimal empty-list handler at
	// GET /api/v1/conversations so the frontend does not 404. Set to true
	// ONLY when no peer package supplies the real implementation.
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
	ID                string  `json:"id"`
	ConversationID    string  `json:"conversation_id"`
	SessionID         string  `json:"session_id"`
	ContactID         string  `json:"contact_id"`
	Channel           string  `json:"channel"`
	Provider          string  `json:"provider"`
	Direction         string  `json:"direction"`
	Type              string  `json:"type"`
	Status            string  `json:"status"`
	Sender            string  `json:"sender"`
	Recipient         string  `json:"recipient"`
	ProviderMessageID string  `json:"provider_message_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
	SentAt            *string `json:"sent_at,omitempty"`
	DeliveredAt       *string `json:"delivered_at,omitempty"`
	ReadAt            *string `json:"read_at,omitempty"`
}

// MessageListResponse is the 200 body of GET /api/v1/conversations/{id}/messages.
type MessageListResponse struct {
	Messages   []MessageDTO `json:"messages"`
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
		Messages:   make([]MessageDTO, 0, len(page.Messages)),
		NextCursor: page.NextCursor,
	}
	for _, m := range page.Messages {
		dto.Messages = append(dto.Messages, toMessageDTO(m))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// listConversationsStub returns an empty conversation list. Placeholder that
// unblocks the frontend until the full list impl lands under Phase 1 Task 4.
func (h *messagesHandler) listConversationsStub(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ConversationListResponse{Conversations: []any{}})
}

func (h *messagesHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// selectPayload picks the matching payload sub-object off the SendMessageRequest
// based on Type. Returns the raw JSON bytes to hand to the application layer.
func selectPayload(req SendMessageRequest) ([]byte, error) {
	switch msgdom.Type(req.Type) {
	case msgdom.TypeText:
		if req.Text == nil {
			return nil, errors.New("text payload required for type=text")
		}
		return *req.Text, nil
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
	return dto
}
