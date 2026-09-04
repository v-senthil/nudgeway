package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// ConversationSummary is the row shape returned by ListForOrg — enriched with
// contact display name + last-message preview so the inbox list can render in
// a single round trip.
type ConversationSummary struct {
	ID                 conversation.ID
	OrgID              organization.ID
	ContactID          string
	ContactDisplay     string
	Type               conversation.Type
	GroupID            string
	GroupSubject       string
	Status             conversation.Status
	Channel            string
	LastMessageAt      *time.Time
	LastMessagePreview string
	UnreadCount        int
	CreatedAt          time.Time
}

// ListForOrg returns every conversation belonging to org, ordered newest
// last-message-first. Joins contacts + messages to populate the display
// name + last-message preview so the inbox list renders in one hop.
//
// Bounded at 200 rows per call for the walking-skeleton path — real
// pagination lands with the search + filter work in Phase 2.
func (c *Conversations) ListForOrg(ctx context.Context, orgID organization.ID) ([]ConversationSummary, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, fmt.Errorf("conversations list: %w", err)
	}
	// Latest-message subquery drives both the preview text and the
	// last_message_at fallback — conversations.last_message_at is not
	// populated by the InboundService today; the freshest signal is the
	// max(created_at) across the conversation's rows. Preview text is
	// pulled from the newest message's JSON metadata.text (populated by
	// the InboundService when the payload was a text/media message).
	const q = `
		SELECT
		    c.id,
		    c.contact_id,
		    COALESCE(ct.display_name, '') AS contact_display,
		    c.type,
		    c.group_id,
		    COALESCE(g.subject, '') AS group_subject,
		    c.status,
		    COALESCE(be.channel, '') AS channel,
		    COALESCE(c.last_message_at,
		             (SELECT MAX(m.created_at) FROM messages m
		                WHERE m.org_id = c.org_id AND m.conversation_id = c.id)
		            ) AS last_message_at,
		    COALESCE(
		      (SELECT JSON_UNQUOTE(JSON_EXTRACT(m.metadata, '$.text'))
		         FROM messages m
		         WHERE m.org_id = c.org_id AND m.conversation_id = c.id
		         ORDER BY m.created_at DESC LIMIT 1),
		      ''
		    ) AS last_preview,
		    c.unread_count,
		    c.created_at
		  FROM conversations c
		  LEFT JOIN contacts ct         ON ct.id = c.contact_id AND ct.org_id = c.org_id
		  LEFT JOIN sessions_comm s     ON s.id  = c.session_id AND s.org_id  = c.org_id
		  LEFT JOIN business_endpoints be ON be.id = s.business_endpoint_id AND be.org_id = s.org_id
		  LEFT JOIN ` + "`groups`" + ` g ON g.id  = c.group_id   AND g.org_id  = c.org_id
		  WHERE c.org_id = ?
		  ORDER BY COALESCE(c.last_message_at,
		                     (SELECT MAX(m.created_at) FROM messages m
		                        WHERE m.org_id = c.org_id AND m.conversation_id = c.id),
		                     c.created_at) DESC
		  LIMIT 200`
	rows, err := c.db.QueryContext(ctx, q, orgBytes)
	if err != nil {
		return nil, fmt.Errorf("conversations list query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ConversationSummary{}
	for rows.Next() {
		var (
			idBytes                    []byte
			contactBytes, groupBytes   []byte
			display, convType, subject string
			status, ch                 string
			lastAt                     sql.NullTime
			preview                    sql.NullString
			unread                     int
			created                    time.Time
		)
		if err := rows.Scan(&idBytes, &contactBytes, &display, &convType, &groupBytes, &subject, &status, &ch, &lastAt, &preview, &unread, &created); err != nil {
			return nil, fmt.Errorf("conversations list scan: %w", err)
		}
		id, err := ulidFromBytes(idBytes)
		if err != nil {
			return nil, fmt.Errorf("conversations list bad id: %w", err)
		}
		var cid string
		if len(contactBytes) == 16 {
			cid, err = ulidFromBytes(contactBytes)
			if err != nil {
				return nil, fmt.Errorf("conversations list bad contact id: %w", err)
			}
		}
		var gid string
		if len(groupBytes) == 16 {
			gid, err = ulidFromBytes(groupBytes)
			if err != nil {
				return nil, fmt.Errorf("conversations list bad group id: %w", err)
			}
		}
		if convType == "" {
			convType = string(conversation.TypeOneToOne)
		}
		summary := ConversationSummary{
			ID:                 conversation.ID(id),
			OrgID:              orgID,
			ContactID:          cid,
			ContactDisplay:     display,
			Type:               conversation.Type(convType),
			GroupID:            gid,
			GroupSubject:       subject,
			Status:             conversation.Status(status),
			Channel:            ch,
			LastMessagePreview: preview.String,
			UnreadCount:        unread,
			CreatedAt:          created,
		}
		if lastAt.Valid {
			t := lastAt.Time
			summary.LastMessageAt = &t
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conversations list rows: %w", err)
	}
	return out, nil
}
