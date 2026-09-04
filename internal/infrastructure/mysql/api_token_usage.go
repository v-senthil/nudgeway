package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/apitokenusage"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// APITokenUsage implements repository.APITokenUsageRepo against the
// api_token_usage + api_token_usage_daily tables.
type APITokenUsage struct {
	db *sql.DB
}

// NewAPITokenUsage constructs an APITokenUsage repository against db.
func NewAPITokenUsage(db *sql.DB) *APITokenUsage { return &APITokenUsage{db: db} }

// Record inserts one usage row. OrgID + TokenID are required — a bad
// wire-up surfaces loudly as an ErrInvalidEntry rather than a silent row
// we can't attribute. OccurredAt defaults to time.Now().UTC() when zero.
func (r *APITokenUsage) Record(ctx context.Context, e apitokenusage.Entry) error {
	if e.OrgID == "" || e.TokenID == "" {
		return fmt.Errorf("api_token_usage: %w", apitokenusage.ErrInvalidEntry)
	}
	if e.ID == "" {
		e.ID = apitokenusage.NewID()
	}
	idBytes, err := ulidToBytes(string(e.ID))
	if err != nil {
		return fmt.Errorf("api_token_usage id: %w", err)
	}
	orgBytes, err := ulidToBytes(string(e.OrgID))
	if err != nil {
		return fmt.Errorf("api_token_usage org: %w", err)
	}
	tokenBytes, err := ulidToBytes(string(e.TokenID))
	if err != nil {
		return fmt.Errorf("api_token_usage token: %w", err)
	}
	occurred := e.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	} else {
		occurred = occurred.UTC()
	}
	var reqBody, resBody any
	if len(e.RequestBody) > 0 {
		reqBody = e.RequestBody
	}
	if len(e.ResponseBody) > 0 {
		resBody = e.ResponseBody
	}
	var userAgent any
	if e.UserAgent != "" {
		userAgent = truncateString(e.UserAgent, 500)
	}
	var errMsg any
	if e.ErrorMessage != "" {
		errMsg = truncateString(e.ErrorMessage, 1000)
	}

	const q = `INSERT INTO api_token_usage
	    (id, org_id, token_id, occurred_at, request_id, method, path,
	     status_code, latency_ms, remote_ip, user_agent,
	     request_body, response_body, request_bytes, response_bytes,
	     error_message)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q,
		idBytes, orgBytes, tokenBytes, occurred,
		truncateString(e.RequestID, 64),
		truncateString(e.Method, 10),
		truncateString(e.Path, 500),
		e.StatusCode, e.LatencyMs,
		truncateString(e.RemoteIP, 45),
		userAgent,
		reqBody, resBody, e.RequestBytes, e.ResponseBytes,
		errMsg,
	); err != nil {
		return fmt.Errorf("api_token_usage insert: %w", err)
	}
	return nil
}

// List returns a page of usage entries newest-first for org. Pagination
// uses the (occurred_at, id) tuple encoded as an opaque base64 cursor.
func (r *APITokenUsage) List(
	ctx context.Context,
	orgID organization.ID,
	filter repository.UsageFilter,
) ([]apitokenusage.Entry, string, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return nil, "", fmt.Errorf("api_token_usage org: %w", err)
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

	if filter.TokenID != nil {
		b, err := ulidToBytes(string(*filter.TokenID))
		if err != nil {
			return nil, "", fmt.Errorf("api_token_usage token filter: %w", err)
		}
		conds = append(conds, "token_id = ?")
		args = append(args, b)
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
		cAt, cID, err := decodeAPITokenUsageCursor(filter.Cursor)
		if err != nil {
			return nil, "", err
		}
		conds = append(conds, "(occurred_at, id) < (?, ?)")
		args = append(args, cAt.UTC(), cID)
	}

	q := "SELECT id, org_id, token_id, occurred_at, request_id, method, path, " +
		"status_code, latency_ms, remote_ip, user_agent, request_body, response_body, " +
		"request_bytes, response_bytes, error_message " +
		"FROM api_token_usage WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY occurred_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("api_token_usage list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]apitokenusage.Entry, 0, limit)
	for rows.Next() {
		e, err := scanAPITokenUsage(rows.Scan)
		if err != nil {
			return nil, "", err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("api_token_usage rows: %w", err)
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		out = out[:limit]
		nextIDBytes, err := ulidToBytes(string(last.ID))
		if err != nil {
			return nil, "", fmt.Errorf("api_token_usage next cursor: %w", err)
		}
		next = encodeAPITokenUsageCursor(last.OccurredAt, nextIDBytes)
	}
	return out, next, nil
}

// Metrics returns the aggregate KPIs, per-day series, per-status histogram,
// and top-paths breakdown for one token over [from, to]. Series data
// comes from the rollup table; KPIs + top-paths + status histogram come
// from the raw log so late-arriving traffic (before the next rollup
// tick) is reflected immediately.
func (r *APITokenUsage) Metrics(
	ctx context.Context,
	orgID organization.ID,
	tokenID apitoken.ID,
	from, to time.Time,
) (apitokenusage.Metrics, error) {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage metrics org: %w", err)
	}
	tokenBytes, err := ulidToBytes(string(tokenID))
	if err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage metrics token: %w", err)
	}
	if from.IsZero() {
		from = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	from = from.UTC()
	to = to.UTC()

	metrics := apitokenusage.Metrics{
		ByStatus: map[int]int64{},
		ByDay:    []apitokenusage.DailyPoint{},
		TopPaths: []apitokenusage.PathHit{},
	}

	// --- KPIs from the raw log ---------------------------------------------
	const kpiQ = `SELECT COUNT(*),
	                     SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END),
	                     COALESCE(AVG(latency_ms), 0),
	                     COALESCE(SUM(request_bytes), 0),
	                     COALESCE(SUM(response_bytes), 0)
	              FROM api_token_usage
	              WHERE org_id = ? AND token_id = ?
	                AND occurred_at >= ? AND occurred_at < ?`
	var total, errs sql.NullInt64
	var avgLat sql.NullFloat64
	var bIn, bOut sql.NullInt64
	if err := r.db.QueryRowContext(ctx, kpiQ, orgBytes, tokenBytes, from, to).
		Scan(&total, &errs, &avgLat, &bIn, &bOut); err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage kpi: %w", err)
	}
	metrics.TotalRequests = total.Int64
	metrics.ErrorCount = errs.Int64
	metrics.AvgLatencyMs = int64(avgLat.Float64)
	metrics.BytesIn = bIn.Int64
	metrics.BytesOut = bOut.Int64

	// --- Per-day series from the rollup table ------------------------------
	// Cast the window to DATE so we compare against the daily granularity.
	const seriesQ = `SELECT day, total_requests, error_count, avg_latency_ms,
	                        bytes_in_total, bytes_out_total
	                 FROM api_token_usage_daily
	                 WHERE org_id = ? AND token_id = ?
	                   AND day >= DATE(?) AND day <= DATE(?)
	                 ORDER BY day ASC`
	rows, err := r.db.QueryContext(ctx, seriesQ, orgBytes, tokenBytes, from, to)
	if err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage series: %w", err)
	}
	for rows.Next() {
		var (
			day                            time.Time
			totalReq, errCnt, avgL         int64
			bytesInTotal, bytesOutTotal    int64
		)
		if err := rows.Scan(&day, &totalReq, &errCnt, &avgL, &bytesInTotal, &bytesOutTotal); err != nil {
			_ = rows.Close()
			return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage series scan: %w", err)
		}
		metrics.ByDay = append(metrics.ByDay, apitokenusage.DailyPoint{
			Day:           day,
			TotalRequests: totalReq,
			ErrorCount:    errCnt,
			AvgLatencyMs:  avgL,
			BytesIn:       bytesInTotal,
			BytesOut:      bytesOutTotal,
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage series rows: %w", err)
	}
	_ = rows.Close()

	// --- Status histogram from the raw log ---------------------------------
	const statusQ = `SELECT status_code, COUNT(*)
	                 FROM api_token_usage
	                 WHERE org_id = ? AND token_id = ?
	                   AND occurred_at >= ? AND occurred_at < ?
	                 GROUP BY status_code`
	sRows, err := r.db.QueryContext(ctx, statusQ, orgBytes, tokenBytes, from, to)
	if err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage status: %w", err)
	}
	for sRows.Next() {
		var code int
		var count int64
		if err := sRows.Scan(&code, &count); err != nil {
			_ = sRows.Close()
			return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage status scan: %w", err)
		}
		metrics.ByStatus[code] = count
	}
	if err := sRows.Err(); err != nil {
		_ = sRows.Close()
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage status rows: %w", err)
	}
	_ = sRows.Close()

	// --- Top paths from the raw log ----------------------------------------
	const pathQ = `SELECT path, COUNT(*) AS c
	               FROM api_token_usage
	               WHERE org_id = ? AND token_id = ?
	                 AND occurred_at >= ? AND occurred_at < ?
	               GROUP BY path
	               ORDER BY c DESC
	               LIMIT 10`
	pRows, err := r.db.QueryContext(ctx, pathQ, orgBytes, tokenBytes, from, to)
	if err != nil {
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage paths: %w", err)
	}
	for pRows.Next() {
		var p string
		var c int64
		if err := pRows.Scan(&p, &c); err != nil {
			_ = pRows.Close()
			return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage paths scan: %w", err)
		}
		metrics.TopPaths = append(metrics.TopPaths, apitokenusage.PathHit{Path: p, Count: c})
	}
	if err := pRows.Err(); err != nil {
		_ = pRows.Close()
		return apitokenusage.Metrics{}, fmt.Errorf("api_token_usage paths rows: %w", err)
	}
	_ = pRows.Close()

	// Keep by-day series deterministically sorted (defensive — SQL ORDER
	// BY already guarantees it, but the API contract is "ascending").
	sort.Slice(metrics.ByDay, func(i, j int) bool {
		return metrics.ByDay[i].Day.Before(metrics.ByDay[j].Day)
	})
	return metrics, nil
}

// RollupDay recomputes and upserts every api_token_usage_daily row for
// orgID on the given (local-tz) day. Idempotent: repeat calls with the
// same arguments produce the same table state. Uses a single INSERT ...
// SELECT ... ON DUPLICATE KEY UPDATE so the whole rollup is one round-
// trip per day.
func (r *APITokenUsage) RollupDay(
	ctx context.Context,
	orgID organization.ID,
	day time.Time,
) error {
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("api_token_usage rollup org: %w", err)
	}
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	const q = `INSERT INTO api_token_usage_daily
	    (org_id, token_id, day,
	     total_requests, error_count, avg_latency_ms,
	     bytes_in_total, bytes_out_total)
	  SELECT org_id, token_id, DATE(occurred_at),
	         COUNT(*),
	         SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END),
	         CAST(COALESCE(AVG(latency_ms), 0) AS SIGNED),
	         COALESCE(SUM(request_bytes), 0),
	         COALESCE(SUM(response_bytes), 0)
	  FROM api_token_usage
	  WHERE org_id = ? AND occurred_at >= ? AND occurred_at < ?
	  GROUP BY org_id, token_id, DATE(occurred_at)
	  ON DUPLICATE KEY UPDATE
	    total_requests  = VALUES(total_requests),
	    error_count     = VALUES(error_count),
	    avg_latency_ms  = VALUES(avg_latency_ms),
	    bytes_in_total  = VALUES(bytes_in_total),
	    bytes_out_total = VALUES(bytes_out_total)`
	if _, err := r.db.ExecContext(ctx, q, orgBytes, dayStart, dayEnd); err != nil {
		return fmt.Errorf("api_token_usage rollup exec: %w", err)
	}
	return nil
}

// scanAPITokenUsage decodes one api_token_usage row into a domain Entry.
func scanAPITokenUsage(scan func(dest ...any) error) (apitokenusage.Entry, error) {
	var (
		idB, orgB, tokenB []byte
		occurred          time.Time
		requestID         string
		method, path      string
		statusCode        int
		latencyMs         int
		remoteIP          string
		userAgent         sql.NullString
		reqBody, resBody  []byte
		requestBytes      int
		responseBytes     int
		errMsg            sql.NullString
	)
	if err := scan(&idB, &orgB, &tokenB, &occurred, &requestID, &method, &path,
		&statusCode, &latencyMs, &remoteIP, &userAgent, &reqBody, &resBody,
		&requestBytes, &responseBytes, &errMsg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apitokenusage.Entry{}, apitokenusage.ErrNotFound
		}
		return apitokenusage.Entry{}, fmt.Errorf("api_token_usage scan: %w", err)
	}
	idStr, err := ulidFromBytes(idB)
	if err != nil {
		return apitokenusage.Entry{}, fmt.Errorf("api_token_usage bad id: %w", err)
	}
	orgStr, err := ulidFromBytes(orgB)
	if err != nil {
		return apitokenusage.Entry{}, fmt.Errorf("api_token_usage bad org: %w", err)
	}
	tokenStr, err := ulidFromBytes(tokenB)
	if err != nil {
		return apitokenusage.Entry{}, fmt.Errorf("api_token_usage bad token: %w", err)
	}
	e := apitokenusage.Entry{
		ID:            apitokenusage.ID(idStr),
		OrgID:         organization.ID(orgStr),
		TokenID:       apitoken.ID(tokenStr),
		OccurredAt:    occurred,
		RequestID:     requestID,
		Method:        method,
		Path:          path,
		StatusCode:    statusCode,
		LatencyMs:     latencyMs,
		RemoteIP:      remoteIP,
		RequestBytes:  requestBytes,
		ResponseBytes: responseBytes,
	}
	if userAgent.Valid {
		e.UserAgent = userAgent.String
	}
	if errMsg.Valid {
		e.ErrorMessage = errMsg.String
	}
	if len(reqBody) > 0 {
		e.RequestBody = append([]byte(nil), reqBody...)
	}
	if len(resBody) > 0 {
		e.ResponseBody = append([]byte(nil), resBody...)
	}
	return e, nil
}

// encodeAPITokenUsageCursor packs (occurred_at, id_bytes) into an opaque
// base64 token. Plain form: "<unix_micros>|<hex_id>".
func encodeAPITokenUsageCursor(at time.Time, idBytes []byte) string {
	plain := strconv.FormatInt(at.UTC().UnixMicro(), 10) + "|" +
		base64.RawURLEncoding.EncodeToString(idBytes)
	return base64.RawURLEncoding.EncodeToString([]byte(plain))
}

// decodeAPITokenUsageCursor unpacks an opaque cursor.
func decodeAPITokenUsageCursor(cursor string) (time.Time, []byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("api_token_usage: invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, nil, fmt.Errorf("api_token_usage: invalid cursor shape")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("api_token_usage: invalid cursor ts")
	}
	idBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("api_token_usage: invalid cursor id")
	}
	return time.UnixMicro(micros).UTC(), idBytes, nil
}
