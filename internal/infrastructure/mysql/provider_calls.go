package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/providercall"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ProviderCalls implements repository.ProviderCallRepo against the
// provider_calls table declared in migrations/20260904000001_provider_calls.
type ProviderCalls struct {
	db *sql.DB
}

// NewProviderCalls constructs a ProviderCalls repository against db.
func NewProviderCalls(db *sql.DB) *ProviderCalls { return &ProviderCalls{db: db} }

// Record inserts a new entry and returns its assigned auto-increment ID.
//
// OrgID and Provider are required — bad wire-ups surface loudly as an
// InvalidEntry error rather than a silent row we can't attribute. OccurredAt
// defaults to time.Now().UTC() when the caller left it zero.
func (p *ProviderCalls) Record(ctx context.Context, entry providercall.Entry) (uint64, error) {
	if entry.OrgID == "" || entry.Provider == "" {
		return 0, errors.New("provider_calls: org_id and provider required")
	}
	orgBytes, err := ulidToBytes(entry.OrgID)
	if err != nil {
		return 0, fmt.Errorf("provider_calls org: %w", err)
	}
	var integBytes any
	if entry.IntegrationID != "" {
		b, err := ulidToBytes(entry.IntegrationID)
		if err != nil {
			return 0, fmt.Errorf("provider_calls integration: %w", err)
		}
		integBytes = b
	}
	occurred := entry.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	} else {
		occurred = occurred.UTC()
	}
	// request/response bodies land as nil when empty so MySQL stores NULL
	// instead of a zero-length blob (smaller row footprint on GETs).
	var reqBody, resBody any
	if len(entry.RequestBody) > 0 {
		reqBody = entry.RequestBody
	}
	if len(entry.ResponseBody) > 0 {
		resBody = entry.ResponseBody
	}

	const q = `INSERT INTO provider_calls
	    (org_id, integration_id, provider, operation, method, url,
	     status_code, latency_ms, request_body, response_body,
	     error_class, error_message, trace_id, correlation_id, occurred_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := p.db.ExecContext(ctx, q,
		orgBytes, integBytes, entry.Provider, entry.Operation, entry.Method, entry.URL,
		entry.StatusCode, entry.LatencyMs, reqBody, resBody,
		entry.ErrorClass, truncateString(entry.ErrorMessage, 1024), entry.TraceID, entry.CorrelationID, occurred,
	)
	if err != nil {
		return 0, fmt.Errorf("provider_calls insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("provider_calls lastid: %w", err)
	}
	return uint64(id), nil
}

// List returns a page of provider-call entries newest-first for org.
//
// Pagination uses the (occurred_at, id) tuple encoded as an opaque base64
// cursor. The composite (org_id, occurred_at) index drives the sort;
// adding integration_id or status_code predicates falls back on the other
// composite indexes on the same table.
func (p *ProviderCalls) List(
	ctx context.Context,
	orgID organization.ID,
	filter repository.ProviderCallListFilter,
) ([]providercall.Entry, string, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, "", fmt.Errorf("provider_calls org: %w", err)
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

	if filter.IntegrationID != nil {
		b, err := ulidToBytes(string(*filter.IntegrationID))
		if err != nil {
			return nil, "", fmt.Errorf("provider_calls integration filter: %w", err)
		}
		conds = append(conds, "integration_id = ?")
		args = append(args, b)
	}
	if filter.Operation != "" {
		conds = append(conds, "operation = ?")
		args = append(args, filter.Operation)
	}
	if filter.StatusCodeMin > 0 {
		conds = append(conds, "status_code >= ?")
		args = append(args, filter.StatusCodeMin)
	}
	if filter.StatusCodeMax > 0 {
		conds = append(conds, "status_code <= ?")
		args = append(args, filter.StatusCodeMax)
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "occurred_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "occurred_at < ?")
		args = append(args, filter.Until.UTC())
	}
	if filter.Cursor != "" {
		cAt, cID, err := decodeProviderCallCursor(filter.Cursor)
		if err != nil {
			return nil, "", err
		}
		conds = append(conds, "(occurred_at, id) < (?, ?)")
		args = append(args, cAt.UTC(), cID)
	}

	q := "SELECT id, org_id, integration_id, provider, operation, method, url, " +
		"status_code, latency_ms, request_body, response_body, error_class, " +
		"error_message, trace_id, correlation_id, occurred_at " +
		"FROM provider_calls WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY occurred_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("provider_calls list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]providercall.Entry, 0, limit)
	for rows.Next() {
		e, err := scanProviderCall(rows.Scan)
		if err != nil {
			return nil, "", err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("provider_calls rows: %w", err)
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		out = out[:limit]
		next = encodeProviderCallCursor(last.OccurredAt, last.ID)
	}
	return out, next, nil
}

// scanProviderCall decodes one provider_calls row into a domain Entry.
func scanProviderCall(scan func(dest ...any) error) (providercall.Entry, error) {
	var (
		id                     uint64
		orgB, integB           []byte
		provider, operation    string
		method, url            string
		statusCode             int
		latencyMs              int64
		reqBody, resBody       []byte
		errClass, errMsg       string
		traceID, correlationID string
		occurred               time.Time
	)
	if err := scan(&id, &orgB, &integB, &provider, &operation, &method, &url,
		&statusCode, &latencyMs, &reqBody, &resBody, &errClass, &errMsg,
		&traceID, &correlationID, &occurred); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return providercall.Entry{}, providercall.ErrNotFound
		}
		return providercall.Entry{}, fmt.Errorf("provider_calls scan: %w", err)
	}
	orgStr, err := ulidFromBytes(orgB)
	if err != nil {
		return providercall.Entry{}, fmt.Errorf("provider_calls bad org: %w", err)
	}
	e := providercall.Entry{
		ID:            id,
		OrgID:         orgStr,
		Provider:      provider,
		Operation:     operation,
		Direction:     providercall.DirectionOutbound,
		Method:        method,
		URL:           url,
		StatusCode:    statusCode,
		LatencyMs:     latencyMs,
		ErrorClass:    errClass,
		ErrorMessage:  errMsg,
		TraceID:       traceID,
		CorrelationID: correlationID,
		OccurredAt:    occurred,
	}
	if len(integB) == 16 {
		s, err := ulidFromBytes(integB)
		if err != nil {
			return providercall.Entry{}, fmt.Errorf("provider_calls bad integration: %w", err)
		}
		e.IntegrationID = s
	}
	if len(reqBody) > 0 {
		e.RequestBody = append([]byte(nil), reqBody...)
	}
	if len(resBody) > 0 {
		e.ResponseBody = append([]byte(nil), resBody...)
	}
	return e, nil
}

// encodeProviderCallCursor packs (occurred_at, id) into an opaque base64
// token. Plain form: "<unix_micros>|<id>" — deliberately opaque so
// frontends can't forge or read it.
func encodeProviderCallCursor(at time.Time, id uint64) string {
	plain := strconv.FormatInt(at.UTC().UnixMicro(), 10) + "|" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

// decodeProviderCallCursor unpacks an opaque cursor.
func decodeProviderCallCursor(cursor string) (time.Time, uint64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("provider_calls: invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("provider_calls: invalid cursor shape")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("provider_calls: invalid cursor ts")
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("provider_calls: invalid cursor id")
	}
	return time.UnixMicro(micros).UTC(), id, nil
}

// truncateString returns s bounded at max runes-approximated-as-bytes.
// The ErrorMessage column is VARCHAR(1024); any long provider message gets
// clipped so INSERTs never fail with ER_DATA_TOO_LONG.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
