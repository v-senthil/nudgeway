package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fullwa/fullwa/internal/domain/call"
	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// Calls implements repository.CallRepo against the `calls` table declared
// in migrations/20260904000005_calls.up.sql.
type Calls struct {
	db *sql.DB
}

// NewCalls constructs a Calls repository.
func NewCalls(db *sql.DB) *Calls { return &Calls{db: db} }

// Create inserts a new call row. Returns a wrapped error containing
// ErrDuplicateCall when the (org, provider, provider_call_id) unique
// index rejects the insert.
func (c *Calls) Create(ctx context.Context, row call.Call) error {
	if row.OrgID == "" || row.Provider == "" || row.ProviderCallID == "" {
		return fmt.Errorf("%w: org_id + provider + provider_call_id required", call.ErrInvalid)
	}
	idBytes, err := ulidToBytes(string(row.ID))
	if err != nil {
		return fmt.Errorf("calls id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(row.OrgID))
	if err != nil {
		return fmt.Errorf("calls org: %w", err)
	}
	integBytes, err := ulidToBytes(row.IntegrationID)
	if err != nil {
		return fmt.Errorf("calls integration: %w", err)
	}

	var bepBytes, ctBytes, sidBytes, convBytes any
	if row.BusinessEndpointID != nil {
		b, err := ulidToBytes(string(*row.BusinessEndpointID))
		if err != nil {
			return fmt.Errorf("calls endpoint: %w", err)
		}
		bepBytes = b
	}
	if row.ContactID != nil {
		b, err := ulidToBytes(string(*row.ContactID))
		if err != nil {
			return fmt.Errorf("calls contact: %w", err)
		}
		ctBytes = b
	}
	if row.SessionID != nil {
		b, err := ulidToBytes(string(*row.SessionID))
		if err != nil {
			return fmt.Errorf("calls session: %w", err)
		}
		sidBytes = b
	}
	if row.ConversationID != nil {
		b, err := ulidToBytes(string(*row.ConversationID))
		if err != nil {
			return fmt.Errorf("calls conversation: %w", err)
		}
		convBytes = b
	}
	var startedAt, answeredAt, endedAt any
	if row.StartedAt != nil {
		startedAt = row.StartedAt.UTC()
	}
	if row.AnsweredAt != nil {
		answeredAt = row.AnsweredAt.UTC()
	}
	if row.EndedAt != nil {
		endedAt = row.EndedAt.UTC()
	}
	meta := jsonMustMarshal(orEmptyMap(row.Extras))
	var recordingURL any
	if row.RecordingURL != "" {
		recordingURL = row.RecordingURL
	}

	const q = `INSERT INTO calls
	    (id, org_id, integration_id, business_endpoint_id, contact_id,
	     session_id, conversation_id, provider, provider_call_id, direction,
	     status, from_number, to_number, from_user_id, to_user_id,
	     started_at, answered_at, ended_at, duration_seconds, hangup_reason,
	     recording_url, transcription_ref, metadata)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = c.db.ExecContext(ctx, q,
		idBytes, orgBytes, integBytes, bepBytes, ctBytes,
		sidBytes, convBytes, row.Provider, row.ProviderCallID, string(row.Direction),
		string(row.Status), row.From, row.To, row.FromUserID, row.ToUserID,
		startedAt, answeredAt, endedAt, row.DurationSeconds, row.HangupReason,
		recordingURL, row.TranscriptionRef, meta,
	)
	if err != nil {
		return fmt.Errorf("calls insert: %w", err)
	}
	return nil
}

// UpsertByProviderID inserts on absent, updates on present. Preserves
// already-set timestamps and IDs by using COALESCE.
func (c *Calls) UpsertByProviderID(ctx context.Context, row call.Call) (call.Call, error) {
	if row.OrgID == "" || row.Provider == "" || row.ProviderCallID == "" {
		return call.Call{}, fmt.Errorf("%w: org + provider + provider_call_id required", call.ErrInvalid)
	}
	// Check if a row already exists.
	existing, err := c.getByProviderID(ctx, row.OrgID, row.Provider, row.ProviderCallID)
	if err == nil {
		// Row exists — merge onto it via UPDATE.
		if err := c.mergeUpdate(ctx, existing, row); err != nil {
			return call.Call{}, err
		}
		merged, err := c.Get(ctx, row.OrgID, existing.ID)
		if err != nil {
			return call.Call{}, err
		}
		return merged, nil
	}
	if !errors.Is(err, call.ErrNotFound) {
		return call.Call{}, err
	}
	// Row is absent — insert.
	if err := c.Create(ctx, row); err != nil {
		return call.Call{}, err
	}
	saved, err := c.Get(ctx, row.OrgID, row.ID)
	if err != nil {
		return call.Call{}, err
	}
	return saved, nil
}

// mergeUpdate advances the existing row with values from incoming that
// materially differ. Preserves already-stamped timestamps.
func (c *Calls) mergeUpdate(ctx context.Context, existing, incoming call.Call) error {
	orgBytes, err := ulidToBytes(string(existing.OrgID))
	if err != nil {
		return fmt.Errorf("calls merge org: %w", err)
	}
	idBytes, err := ulidToBytes(string(existing.ID))
	if err != nil {
		return fmt.Errorf("calls merge id: %w", err)
	}
	sets := []string{}
	args := []any{}
	if incoming.Status != "" && incoming.Status != existing.Status {
		sets = append(sets, "status = ?")
		args = append(args, string(incoming.Status))
	}
	if incoming.From != "" && existing.From == "" {
		sets = append(sets, "from_number = ?")
		args = append(args, incoming.From)
	}
	if incoming.To != "" && existing.To == "" {
		sets = append(sets, "to_number = ?")
		args = append(args, incoming.To)
	}
	if incoming.FromUserID != "" && existing.FromUserID == "" {
		sets = append(sets, "from_user_id = ?")
		args = append(args, incoming.FromUserID)
	}
	if incoming.ToUserID != "" && existing.ToUserID == "" {
		sets = append(sets, "to_user_id = ?")
		args = append(args, incoming.ToUserID)
	}
	if incoming.StartedAt != nil && existing.StartedAt == nil {
		sets = append(sets, "started_at = ?")
		args = append(args, incoming.StartedAt.UTC())
	}
	if incoming.AnsweredAt != nil && existing.AnsweredAt == nil {
		sets = append(sets, "answered_at = ?")
		args = append(args, incoming.AnsweredAt.UTC())
	}
	if incoming.EndedAt != nil && existing.EndedAt == nil {
		sets = append(sets, "ended_at = ?")
		args = append(args, incoming.EndedAt.UTC())
	}
	if incoming.DurationSeconds > existing.DurationSeconds {
		sets = append(sets, "duration_seconds = ?")
		args = append(args, incoming.DurationSeconds)
	}
	if incoming.HangupReason != "" && existing.HangupReason == "" {
		sets = append(sets, "hangup_reason = ?")
		args = append(args, incoming.HangupReason)
	}
	if incoming.RecordingURL != "" && existing.RecordingURL == "" {
		sets = append(sets, "recording_url = ?")
		args = append(args, incoming.RecordingURL)
	}
	if incoming.TranscriptionRef != "" && existing.TranscriptionRef == "" {
		sets = append(sets, "transcription_ref = ?")
		args = append(args, incoming.TranscriptionRef)
	}
	if incoming.BusinessEndpointID != nil && existing.BusinessEndpointID == nil {
		b, err := ulidToBytes(string(*incoming.BusinessEndpointID))
		if err == nil {
			sets = append(sets, "business_endpoint_id = ?")
			args = append(args, b)
		}
	}
	if incoming.ContactID != nil && existing.ContactID == nil {
		b, err := ulidToBytes(string(*incoming.ContactID))
		if err == nil {
			sets = append(sets, "contact_id = ?")
			args = append(args, b)
		}
	}
	if incoming.IntegrationID != "" && existing.IntegrationID == "" {
		b, err := ulidToBytes(incoming.IntegrationID)
		if err == nil {
			sets = append(sets, "integration_id = ?")
			args = append(args, b)
		}
	}
	// Merge extras/metadata: preserve existing keys, layer new ones on top.
	// Needed so a later webhook (e.g. status update) doesn't blank an
	// already-stored session_sdp captured on the initial connect event.
	if len(incoming.Extras) > 0 {
		merged := map[string]any{}
		for k, v := range existing.Extras {
			merged[k] = v
		}
		changed := false
		for k, v := range incoming.Extras {
			if _, exists := merged[k]; !exists {
				merged[k] = v
				changed = true
			}
		}
		if changed {
			sets = append(sets, "metadata = ?")
			args = append(args, jsonMustMarshal(merged))
		}
	}
	if len(sets) == 0 {
		return nil
	}
	q := "UPDATE calls SET " + strings.Join(sets, ", ") + " WHERE org_id = ? AND id = ?"
	args = append(args, orgBytes, idBytes)
	if _, err := c.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("calls update: %w", err)
	}
	return nil
}

// Get fetches one row by (org, id).
func (c *Calls) Get(ctx context.Context, orgID organization.ID, id call.ID) (call.Call, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return call.Call{}, fmt.Errorf("calls org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return call.Call{}, fmt.Errorf("calls id: %w", err)
	}
	row := c.db.QueryRowContext(ctx, callSelectSQL+" WHERE org_id = ? AND id = ?", orgBytes, idBytes)
	return scanCall(row.Scan)
}

// getByProviderID resolves a row by (org, provider, provider_call_id).
func (c *Calls) getByProviderID(ctx context.Context, orgID organization.ID, provider, providerCallID string) (call.Call, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return call.Call{}, fmt.Errorf("calls org: %w", err)
	}
	row := c.db.QueryRowContext(ctx,
		callSelectSQL+" WHERE org_id = ? AND provider = ? AND provider_call_id = ?",
		orgBytes, provider, providerCallID,
	)
	return scanCall(row.Scan)
}

// List returns a page of rows newest-first.
func (c *Calls) List(ctx context.Context, orgID organization.ID, filter repository.CallListFilter) (repository.CallPage, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return repository.CallPage{}, fmt.Errorf("calls org: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	conds := []string{"org_id = ?"}
	args := []any{orgBytes}
	if filter.Direction != "" {
		conds = append(conds, "direction = ?")
		args = append(args, string(filter.Direction))
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.ContactID != nil {
		b, err := ulidToBytes(string(*filter.ContactID))
		if err != nil {
			return repository.CallPage{}, fmt.Errorf("calls contact filter: %w", err)
		}
		conds = append(conds, "contact_id = ?")
		args = append(args, b)
	}
	if filter.ConversationID != nil {
		b, err := ulidToBytes(string(*filter.ConversationID))
		if err != nil {
			return repository.CallPage{}, fmt.Errorf("calls conversation filter: %w", err)
		}
		conds = append(conds, "conversation_id = ?")
		args = append(args, b)
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "created_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "created_at < ?")
		args = append(args, filter.Until.UTC())
	}
	if filter.Cursor != "" {
		cAt, cIDBytes, err := decodeCallCursor(filter.Cursor)
		if err != nil {
			return repository.CallPage{}, err
		}
		conds = append(conds, "(created_at, id) < (?, ?)")
		args = append(args, cAt.UTC(), cIDBytes)
	}
	q := callSelectSQL + " WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return repository.CallPage{}, fmt.Errorf("calls list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]call.Call, 0, limit)
	for rows.Next() {
		r, err := scanCall(rows.Scan)
		if err != nil {
			return repository.CallPage{}, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return repository.CallPage{}, fmt.Errorf("calls rows: %w", err)
	}
	next := ""
	if len(items) > limit {
		last := items[limit-1]
		items = items[:limit]
		next = encodeCallCursor(last.CreatedAt, last.ID)
	}
	return repository.CallPage{Items: items, NextCursor: next}, nil
}

// UpdateStatus advances status and stamps the appropriate timestamp.
func (c *Calls) UpdateStatus(ctx context.Context, orgID organization.ID, id call.ID, next call.Status, at time.Time) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("calls org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("calls id: %w", err)
	}
	tsCol := statusTimestampCall(next)
	sets := []string{"status = ?"}
	args := []any{string(next)}
	if tsCol != "" {
		sets = append(sets, tsCol+" = COALESCE("+tsCol+", ?)")
		args = append(args, at.UTC())
	}
	q := "UPDATE calls SET " + strings.Join(sets, ", ") + " WHERE org_id = ? AND id = ?"
	args = append(args, orgBytes, idBytes)
	if _, err := c.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("calls update status: %w", err)
	}
	return nil
}

// AttachRecording stamps recording_url + duration_seconds.
func (c *Calls) AttachRecording(ctx context.Context, orgID organization.ID, id call.ID, recordingURL string, durationSeconds int) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("calls org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("calls id: %w", err)
	}
	const q = "UPDATE calls SET recording_url = ?, duration_seconds = GREATEST(duration_seconds, ?) WHERE org_id = ? AND id = ?"
	if _, err := c.db.ExecContext(ctx, q, recordingURL, durationSeconds, orgBytes, idBytes); err != nil {
		return fmt.Errorf("calls attach recording: %w", err)
	}
	return nil
}

// statusTimestampCall picks the DATETIME column to stamp for a given status.
func statusTimestampCall(s call.Status) string {
	switch s {
	case call.StatusRinging:
		return "started_at"
	case call.StatusAnswered, call.StatusInProgress:
		return "answered_at"
	case call.StatusCompleted, call.StatusFailed, call.StatusDeclined,
		call.StatusNoAnswer, call.StatusMissed:
		return "ended_at"
	}
	return ""
}

// callSelectSQL enumerates the columns in a stable order for scanCall.
const callSelectSQL = `SELECT
    id, org_id, integration_id, business_endpoint_id, contact_id,
    session_id, conversation_id, provider, provider_call_id, direction,
    status, from_number, to_number, from_user_id, to_user_id,
    started_at, answered_at, ended_at, duration_seconds, hangup_reason,
    recording_url, transcription_ref, metadata, created_at, updated_at
  FROM calls`

// scanCall decodes one row.
func scanCall(scan func(dest ...any) error) (call.Call, error) {
	var (
		idB, orgB, integB       []byte
		// []byte instead of sql.RawBytes so the same helper works with
		// *sql.Row.Scan (RawBytes is only allowed on Rows.Scan). Nullable
		// columns land as nil / zero-length slice — length checks below
		// still gate on len==16.
		bepB, ctB, sidB, convB  []byte
		provider, providerCID   string
		direction, status       string
		fromN, toN              string
		fromU, toU              string
		startedAt, answeredAt   sql.NullTime
		endedAt                 sql.NullTime
		duration                int
		hangupReason            string
		recordingURL            sql.NullString
		transcriptionRef        string
		metaRaw                 []byte
		createdAt               time.Time
		updatedAt               sql.NullTime
	)
	if err := scan(&idB, &orgB, &integB, &bepB, &ctB,
		&sidB, &convB, &provider, &providerCID, &direction,
		&status, &fromN, &toN, &fromU, &toU,
		&startedAt, &answeredAt, &endedAt, &duration, &hangupReason,
		&recordingURL, &transcriptionRef, &metaRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return call.Call{}, call.ErrNotFound
		}
		return call.Call{}, fmt.Errorf("calls scan: %w", err)
	}
	idStr, err := ulidFromBytes(idB)
	if err != nil {
		return call.Call{}, fmt.Errorf("calls id decode: %w", err)
	}
	orgStr, err := ulidFromBytes(orgB)
	if err != nil {
		return call.Call{}, fmt.Errorf("calls org decode: %w", err)
	}
	out := call.Call{
		ID:               call.ID(idStr),
		OrgID:            organization.ID(orgStr),
		Provider:         provider,
		ProviderCallID:   providerCID,
		Direction:        call.Direction(direction),
		Status:           call.Status(status),
		From:             fromN,
		To:               toN,
		FromUserID:       fromU,
		ToUserID:         toU,
		DurationSeconds:  duration,
		HangupReason:     hangupReason,
		TranscriptionRef: transcriptionRef,
		CreatedAt:        createdAt,
	}
	if len(integB) == 16 {
		s, err := ulidFromBytes(integB)
		if err == nil {
			out.IntegrationID = s
		}
	}
	if len(bepB) == 16 {
		s, err := ulidFromBytes(bepB)
		if err == nil {
			bid := session.BusinessEndpointID(s)
			out.BusinessEndpointID = &bid
		}
	}
	if len(ctB) == 16 {
		s, err := ulidFromBytes(ctB)
		if err == nil {
			cid := contact.ID(s)
			out.ContactID = &cid
		}
	}
	if len(sidB) == 16 {
		s, err := ulidFromBytes(sidB)
		if err == nil {
			sid := session.ID(s)
			out.SessionID = &sid
		}
	}
	if len(convB) == 16 {
		s, err := ulidFromBytes(convB)
		if err == nil {
			cid := conversation.ID(s)
			out.ConversationID = &cid
		}
	}
	if startedAt.Valid {
		t := startedAt.Time
		out.StartedAt = &t
	}
	if answeredAt.Valid {
		t := answeredAt.Time
		out.AnsweredAt = &t
	}
	if endedAt.Valid {
		t := endedAt.Time
		out.EndedAt = &t
	}
	if recordingURL.Valid {
		out.RecordingURL = recordingURL.String
	}
	if updatedAt.Valid {
		t := updatedAt.Time
		out.UpdatedAt = &t
	}
	if len(metaRaw) > 0 {
		out.Extras = map[string]any{}
		_ = unmarshalJSONBytes(metaRaw, &out.Extras)
	}
	return out, nil
}

// unmarshalJSONBytes wraps encoding/json Unmarshal so callers don't
// re-import it for one-liners.
func unmarshalJSONBytes(b []byte, out any) error {
	return json.Unmarshal(b, out)
}

// encodeCallCursor packs (created_at, id) into an opaque base64 token.
func encodeCallCursor(at time.Time, id call.ID) string {
	plain := strconv.FormatInt(at.UTC().UnixMicro(), 10) + "|" + string(id)
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

// decodeCallCursor unpacks the token produced by encodeCallCursor.
// Returns the timestamp and the raw 16-byte ULID for a VARBINARY(16)
// tuple compare.
func decodeCallCursor(cursor string) (time.Time, []byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("calls: invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, nil, fmt.Errorf("calls: invalid cursor shape")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("calls: invalid cursor ts")
	}
	idB, err := ulidToBytes(parts[1])
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("calls: invalid cursor id")
	}
	return time.UnixMicro(micros).UTC(), idB, nil
}
