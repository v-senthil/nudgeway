package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
	dusr "github.com/fullwa/fullwa/internal/domain/user"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// Conversations implements repository.ConversationRepo.
type Conversations struct {
	db *sql.DB
}

// NewConversations constructs a Conversations repository.
func NewConversations(db *sql.DB) *Conversations { return &Conversations{db: db} }

// FindOrCreateOpen returns the newest open/reopened conversation for the
// session, opening a new one if none exists.
func (c *Conversations) FindOrCreateOpen(
	ctx context.Context,
	orgID organization.ID,
	sessionID session.ID,
	contactID contact.ID,
) (conversation.Conversation, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations org: %w", err)
	}
	sidBytes, err := ulidToBytes(string(sessionID))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations session: %w", err)
	}
	ctBytes, err := ulidToBytes(string(contactID))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations contact: %w", err)
	}
	const selectQ = `SELECT ` + conversationCols + `
	                 FROM conversations
	                 WHERE org_id = ? AND session_id = ? AND status IN ('open','pending','reopened')
	                 ORDER BY created_at DESC LIMIT 1`
	row := c.db.QueryRowContext(ctx, selectQ, orgBytes, sidBytes)
	got, err := scanConversation(row.Scan)
	if err == nil {
		return got, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return conversation.Conversation{}, err
	}
	newID := newULID()
	const insertQ = `INSERT INTO conversations
	    (id, org_id, session_id, contact_id, status, priority, unread_count, ai_state, bot_state, tags)
	  VALUES (?, ?, ?, ?, 'open', 'normal', 0, '', '', JSON_ARRAY())`
	if _, err := c.db.ExecContext(ctx, insertQ, newID[:], orgBytes, sidBytes, ctBytes); err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations insert: %w", err)
	}
	// Read back to hydrate DB defaults + timestamps.
	const readBackQ = `SELECT ` + conversationCols + ` FROM conversations WHERE id = ? LIMIT 1`
	row = c.db.QueryRowContext(ctx, readBackQ, newID[:])
	return scanConversation(row.Scan)
}

// Get fetches by (OrgID, ID).
func (c *Conversations) Get(ctx context.Context, orgID organization.ID, id conversation.ID) (conversation.Conversation, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations id: %w", err)
	}
	row := c.db.QueryRowContext(ctx,
		`SELECT `+conversationCols+` FROM conversations WHERE org_id = ? AND id = ? LIMIT 1`,
		orgBytes, idBytes,
	)
	return scanConversation(row.Scan)
}

// UpdateStatus persists a Status transition with derived timestamps.
func (c *Conversations) UpdateStatus(ctx context.Context, orgID organization.ID, id conversation.ID, status conversation.Status) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("conversations org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("conversations id: %w", err)
	}
	now := time.Now().UTC()
	var resolvedAt any
	if status == conversation.StatusResolved {
		resolvedAt = now
	}
	const q = `UPDATE conversations SET status = ?, resolved_at = ? WHERE org_id = ? AND id = ?`
	res, err := c.db.ExecContext(ctx, q, string(status), resolvedAt, orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("conversations status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Assign persists an assignment change.
func (c *Conversations) Assign(ctx context.Context, orgID organization.ID, id conversation.ID, userID *dusr.ID, teamID *conversation.TeamID) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("conversations org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("conversations id: %w", err)
	}
	var uid, tid any
	if userID != nil {
		b, err := ulidToBytes(string(*userID))
		if err != nil {
			return fmt.Errorf("conversations user: %w", err)
		}
		uid = b
	}
	if teamID != nil {
		b, err := ulidToBytes(string(*teamID))
		if err != nil {
			return fmt.Errorf("conversations team: %w", err)
		}
		tid = b
	}
	const q = `UPDATE conversations SET assigned_user_id = ?, assigned_team_id = ? WHERE org_id = ? AND id = ?`
	res, err := c.db.ExecContext(ctx, q, uid, tid, orgBytes, idBytes)
	if err != nil {
		return fmt.Errorf("conversations assign: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForContact returns conversations for a contact, newest first.
func (c *Conversations) ListForContact(ctx context.Context, orgID organization.ID, contactID contact.ID, filter repository.ConversationListFilter) (repository.ConversationPage, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.ConversationPage{}, fmt.Errorf("conversations org: %w", err)
	}
	ctBytes, err := ulidToBytes(string(contactID))
	if err != nil {
		return repository.ConversationPage{}, fmt.Errorf("conversations contact: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conds := []string{"org_id = ?", "contact_id = ?"}
	args := []any{orgBytes, ctBytes}
	if filter.Cursor != "" {
		cur, err := ulidToBytes(filter.Cursor)
		if err != nil {
			return repository.ConversationPage{}, fmt.Errorf("conversations cursor: %w", err)
		}
		conds = append(conds, "id < ?")
		args = append(args, cur)
	}
	if filter.Status != nil {
		conds = append(conds, "status = ?")
		args = append(args, string(*filter.Status))
	}
	sqlStr := "SELECT " + conversationCols + " FROM conversations WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := c.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return repository.ConversationPage{}, fmt.Errorf("conversations list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := repository.ConversationPage{}
	for rows.Next() {
		conv, err := scanConversation(rows.Scan)
		if err != nil {
			return repository.ConversationPage{}, err
		}
		out.Conversations = append(out.Conversations, conv)
	}
	if err := rows.Err(); err != nil {
		return repository.ConversationPage{}, fmt.Errorf("conversations rows: %w", err)
	}
	if len(out.Conversations) > limit {
		last := out.Conversations[limit-1]
		out.Conversations = out.Conversations[:limit]
		out.NextCursor = string(last.ID)
	}
	return out, nil
}

// conversationCols is the canonical SELECT column list used everywhere.
const conversationCols = `id, org_id, session_id, contact_id, status, assigned_user_id, assigned_team_id, priority, unread_count, last_message_at, sla_due_at, ai_state, bot_state, tags, created_at, resolved_at`

// scanConversation decodes a row into conversation.Conversation.
func scanConversation(scan func(dest ...any) error) (conversation.Conversation, error) {
	var (
		id, org, sid, cid   []byte
		aUser, aTeam        []byte
		status              string
		priority            string
		unread              int
		lastMsg, sla, resAt sql.NullTime
		aiState, botState   string
		tagsBytes           []byte
		created             time.Time
	)
	if err := scan(&id, &org, &sid, &cid, &status, &aUser, &aTeam, &priority, &unread, &lastMsg, &sla, &aiState, &botState, &tagsBytes, &created, &resAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.Conversation{}, ErrNotFound
		}
		return conversation.Conversation{}, fmt.Errorf("conversations scan: %w", err)
	}
	idStr, err := ulidFromBytes(id)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(org)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations bad org: %w", err)
	}
	sidStr, err := ulidFromBytes(sid)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations bad session: %w", err)
	}
	cidStr, err := ulidFromBytes(cid)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversations bad contact: %w", err)
	}
	out := conversation.Conversation{
		ID:          conversation.ID(idStr),
		OrgID:       organization.ID(orgStr),
		SessionID:   session.ID(sidStr),
		ContactID:   contact.ID(cidStr),
		Status:      conversation.Status(status),
		Priority:    conversation.Priority(priority),
		UnreadCount: unread,
		AIState:     aiState,
		BotState:    botState,
		CreatedAt:   created,
	}
	if len(aUser) == 16 {
		s, err := ulidFromBytes(aUser)
		if err != nil {
			return conversation.Conversation{}, fmt.Errorf("conversations bad user: %w", err)
		}
		uid := dusr.ID(s)
		out.AssignedUserID = &uid
	}
	if len(aTeam) == 16 {
		s, err := ulidFromBytes(aTeam)
		if err != nil {
			return conversation.Conversation{}, fmt.Errorf("conversations bad team: %w", err)
		}
		tid := conversation.TeamID(s)
		out.AssignedTeamID = &tid
	}
	if lastMsg.Valid {
		t := lastMsg.Time
		out.LastMessageAt = &t
	}
	if sla.Valid {
		t := sla.Time
		out.SLADueAt = &t
	}
	if resAt.Valid {
		t := resAt.Time
		out.ResolvedAt = &t
	}
	if len(tagsBytes) > 0 {
		var tags []string
		if err := json.Unmarshal(tagsBytes, &tags); err == nil {
			out.Tags = tags
		}
	}
	return out, nil
}
