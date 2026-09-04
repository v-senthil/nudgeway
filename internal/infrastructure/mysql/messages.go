package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/message"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ErrDuplicateMessage is returned by Messages.Create when the
// (org_id, provider, provider_message_id) unique index rejects an insert
// as a duplicate. Callers ingesting webhooks use this signal to absorb
// duplicate deliveries as no-ops.
var ErrDuplicateMessage = errors.New("mysql: duplicate message")

// Messages implements repository.MessageRepo.
type Messages struct {
	db *sql.DB
}

// NewMessages constructs a Messages repository.
func NewMessages(db *sql.DB) *Messages { return &Messages{db: db} }

// Create inserts a new message row. Returns ErrDuplicateMessage when the
// (org, provider, provider_message_id) unique index rejects the insert.
func (m *Messages) Create(ctx context.Context, msg message.Message) error {
	idBytes, err := ulidToBytes(string(msg.ID))
	if err != nil {
		return fmt.Errorf("messages id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(msg.OrgID))
	if err != nil {
		return fmt.Errorf("messages org: %w", err)
	}
	ctBytes, err := ulidToBytes(string(msg.ContactID))
	if err != nil {
		return fmt.Errorf("messages contact: %w", err)
	}
	sidBytes, err := ulidToBytes(string(msg.SessionID))
	if err != nil {
		return fmt.Errorf("messages session: %w", err)
	}
	convBytes, err := ulidToBytes(string(msg.ConversationID))
	if err != nil {
		return fmt.Errorf("messages conversation: %w", err)
	}
	var providerMsgID any
	if msg.ProviderMessageID != "" {
		providerMsgID = msg.ProviderMessageID
	}
	var sentAt, deliveredAt, readAt any
	if msg.SentAt != nil {
		sentAt = msg.SentAt.UTC()
	}
	if msg.DeliveredAt != nil {
		deliveredAt = msg.DeliveredAt.UTC()
	}
	if msg.ReadAt != nil {
		readAt = msg.ReadAt.UTC()
	}
	meta := jsonMustMarshal(orEmptyMap(msg.Metadata))
	const q = `INSERT INTO messages
	    (id, org_id, contact_id, session_id, conversation_id, channel, provider, direction,
	     sender_identity, recipient_identity, message_type, provider_message_id, status,
	     sent_at, delivered_at, read_at, payload_ref, metadata)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = m.db.ExecContext(ctx, q,
		idBytes, orgBytes, ctBytes, sidBytes, convBytes,
		msg.Channel, msg.Provider, string(msg.Direction),
		msg.SenderIdentity, msg.RecipientIdentity, string(msg.MessageType), providerMsgID, string(msg.Status),
		sentAt, deliveredAt, readAt, msg.PayloadRef, meta,
	)
	if err != nil {
		if isDuplicateErr(err) {
			return ErrDuplicateMessage
		}
		return fmt.Errorf("messages insert: %w", err)
	}
	return nil
}

// UpdateStatus advances a message's Status by (OrgID, ProviderMessageID)
// — the shape webhook status callbacks arrive in. The internal message
// ID is not required.
//
// The port declares (id message.ID) but persistence lookups happen on
// provider_message_id, so we treat the passed ID as the provider message
// id when it isn't a valid ULID. This is the shape the webhook worker
// wires up; a future refactor can split the two into distinct methods.
func (m *Messages) UpdateStatus(ctx context.Context, orgID organization.ID, id message.ID, next message.Status, at time.Time) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("messages org: %w", err)
	}
	tsCol, err := statusTimestampColumn(next)
	if err != nil {
		return err
	}
	// Try ULID-shape internal id first; fall back to provider_message_id
	// for the webhook-inbound case.
	sets := []string{"status = ?"}
	args := []any{string(next)}
	if tsCol != "" {
		sets = append(sets, tsCol+" = COALESCE("+tsCol+", ?)")
		args = append(args, at.UTC())
	}
	setClause := strings.Join(sets, ", ")

	if idBytes, err := ulidToBytes(string(id)); err == nil {
		q := "UPDATE messages SET " + setClause + " WHERE org_id = ? AND id = ?"
		callArgs := append([]any{}, args...)
		callArgs = append(callArgs, orgBytes, idBytes)
		res, err := m.db.ExecContext(ctx, q, callArgs...)
		if err != nil {
			return fmt.Errorf("messages status by id: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
	}
	q := "UPDATE messages SET " + setClause + " WHERE org_id = ? AND provider_message_id = ?"
	callArgs := append([]any{}, args...)
	callArgs = append(callArgs, orgBytes, string(id))
	if _, err := m.db.ExecContext(ctx, q, callArgs...); err != nil {
		return fmt.Errorf("messages status by provider id: %w", err)
	}
	// Idempotent: not-matching rows are treated as no-op.
	return nil
}

// Get returns a single message row by (org, id). Returns ErrNotFound when
// the row does not exist.
func (m *Messages) Get(ctx context.Context, orgID organization.ID, id message.ID) (message.Message, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return message.Message{}, fmt.Errorf("messages org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return message.Message{}, fmt.Errorf("messages id: %w", err)
	}
	q := "SELECT " + messageCols + " FROM messages WHERE org_id = ? AND id = ? LIMIT 1"
	row := m.db.QueryRowContext(ctx, q, orgBytes, idBytes)
	msg, err := scanMessage(row.Scan)
	if err != nil {
		return message.Message{}, err
	}
	return msg, nil
}

// ListByConversation returns metadata rows for a conversation newest-first.
func (m *Messages) ListByConversation(ctx context.Context, orgID organization.ID, convID conversation.ID, filter repository.MessageListFilter) (repository.MessagePage, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.MessagePage{}, fmt.Errorf("messages org: %w", err)
	}
	cvBytes, err := ulidToBytes(string(convID))
	if err != nil {
		return repository.MessagePage{}, fmt.Errorf("messages conv: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conds := []string{"org_id = ?", "conversation_id = ?"}
	args := []any{orgBytes, cvBytes}
	if filter.Cursor != "" {
		cur, err := ulidToBytes(filter.Cursor)
		if err != nil {
			return repository.MessagePage{}, fmt.Errorf("messages cursor: %w", err)
		}
		conds = append(conds, "id < ?")
		args = append(args, cur)
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "created_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	sqlStr := "SELECT " + messageCols + " FROM messages WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := m.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return repository.MessagePage{}, fmt.Errorf("messages list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := repository.MessagePage{}
	for rows.Next() {
		msg, err := scanMessage(rows.Scan)
		if err != nil {
			return repository.MessagePage{}, err
		}
		out.Messages = append(out.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return repository.MessagePage{}, fmt.Errorf("messages rows: %w", err)
	}
	if len(out.Messages) > limit {
		last := out.Messages[limit-1]
		out.Messages = out.Messages[:limit]
		out.NextCursor = string(last.ID)
	}
	return out, nil
}

// FindByCallID returns the synthetic message row whose metadata.call_id
// matches callID. Returns ErrNotFound when no such row exists. The lookup
// uses MySQL's JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.call_id')) — a
// non-indexed scan by design, since call inline-messages are rare and the
// query is only ever run once per call webhook event.
func (m *Messages) FindByCallID(ctx context.Context, orgID organization.ID, callID string) (message.Message, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return message.Message{}, fmt.Errorf("messages org: %w", err)
	}
	q := "SELECT " + messageCols + " FROM messages" +
		" WHERE org_id = ? AND message_type = 'call'" +
		" AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.call_id')) = ?" +
		" ORDER BY id DESC LIMIT 1"
	row := m.db.QueryRowContext(ctx, q, orgBytes, callID)
	msg, err := scanMessage(row.Scan)
	if err != nil {
		return message.Message{}, err
	}
	return msg, nil
}

// FindByCallIDAndStatus returns the info message row for the given
// (call_id, call_status) tuple. Matches any message_type in ('info','call')
// so pre-existing legacy `call` rows still dedupe. Non-indexed JSON scan by
// design — the query runs at most once per call status webhook.
func (m *Messages) FindByCallIDAndStatus(ctx context.Context, orgID organization.ID, callID, status string) (message.Message, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return message.Message{}, fmt.Errorf("messages org: %w", err)
	}
	q := "SELECT " + messageCols + " FROM messages" +
		" WHERE org_id = ? AND message_type IN ('info','call')" +
		" AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.call_id')) = ?" +
		" AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.call_status')) = ?" +
		" ORDER BY id DESC LIMIT 1"
	row := m.db.QueryRowContext(ctx, q, orgBytes, callID, status)
	msg, err := scanMessage(row.Scan)
	if err != nil {
		return message.Message{}, err
	}
	return msg, nil
}

// statusTimestampColumn maps a message.Status to the timestamp column it
// should stamp. Returns "" for statuses without a dedicated timestamp.
func statusTimestampColumn(s message.Status) (string, error) {
	switch s {
	case message.StatusSent:
		return "sent_at", nil
	case message.StatusDelivered:
		return "delivered_at", nil
	case message.StatusRead:
		return "read_at", nil
	case message.StatusFailed, message.StatusQueued:
		return "", nil
	default:
		return "", fmt.Errorf("messages: unknown status %q", s)
	}
}

// messageCols is the canonical SELECT column list for messages.
const messageCols = `id, org_id, contact_id, session_id, conversation_id, channel, provider, direction, sender_identity, recipient_identity, message_type, provider_message_id, status, created_at, sent_at, delivered_at, read_at, payload_ref, metadata`

// scanMessage decodes a row into message.Message.
func scanMessage(scan func(dest ...any) error) (message.Message, error) {
	var (
		id, org, ct, sid, conv                 []byte
		channel, provider, dir                 string
		sender, recipient, msgType             string
		providerMsgID                          sql.NullString
		status                                 string
		created                                time.Time
		sentAt, deliveredAt, readAt            sql.NullTime
		payloadRef                             string
		metaBytes                              []byte
	)
	if err := scan(&id, &org, &ct, &sid, &conv, &channel, &provider, &dir, &sender, &recipient, &msgType, &providerMsgID, &status, &created, &sentAt, &deliveredAt, &readAt, &payloadRef, &metaBytes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return message.Message{}, ErrNotFound
		}
		return message.Message{}, fmt.Errorf("messages scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return message.Message{}, fmt.Errorf("messages bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return message.Message{}, fmt.Errorf("messages bad org: %w", err)
	}
	ctStr, err := ulidFromBytes(ct)
	if err != nil {
		return message.Message{}, fmt.Errorf("messages bad contact: %w", err)
	}
	sidStr, err := ulidFromBytes(sid)
	if err != nil {
		return message.Message{}, fmt.Errorf("messages bad session: %w", err)
	}
	convStr, err := ulidFromBytes(conv)
	if err != nil {
		return message.Message{}, fmt.Errorf("messages bad conv: %w", err)
	}
	out := message.Message{
		ID:                message.ID(idStr),
		OrgID:             organization.ID(orgStr),
		ContactID:         contact.ID(ctStr),
		SessionID:         session.ID(sidStr),
		ConversationID:    conversation.ID(convStr),
		Channel:           channel,
		Provider:          provider,
		Direction:         message.Direction(dir),
		SenderIdentity:    sender,
		RecipientIdentity: recipient,
		MessageType:       message.Type(msgType),
		Status:            message.Status(status),
		CreatedAt:         created,
		PayloadRef:        payloadRef,
	}
	if providerMsgID.Valid {
		out.ProviderMessageID = providerMsgID.String
	}
	if sentAt.Valid {
		t := sentAt.Time
		out.SentAt = &t
	}
	if deliveredAt.Valid {
		t := deliveredAt.Time
		out.DeliveredAt = &t
	}
	if readAt.Valid {
		t := readAt.Time
		out.ReadAt = &t
	}
	if len(metaBytes) > 0 {
		var m map[string]any
		if err := json.Unmarshal(metaBytes, &m); err != nil {
			return message.Message{}, fmt.Errorf("messages metadata: %w", err)
		}
		out.Metadata = m
	}
	return out, nil
}
