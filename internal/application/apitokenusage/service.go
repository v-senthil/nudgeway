// Package apitokenusage is the application-service layer for the
// per-request execution log recorded against every bearer-authenticated
// call. It owns body truncation, JSON redaction of known secret keys,
// and read-side aggregation on behalf of the REST handler.
package apitokenusage

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/apitokenusage"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// DefaultMaxBodyBytes bounds the persisted length of request / response
// bodies. Kept small enough that a chatty API caller doesn't fill MySQL,
// large enough to include a typical JSON payload with its diagnostics.
const DefaultMaxBodyBytes = 8 << 10 // 8 KiB

// redactedPlaceholder is the sentinel value written in place of a
// redacted JSON value.
const redactedPlaceholder = "[redacted]"

// redactKeys lists JSON keys whose values are always overwritten with
// redactedPlaceholder before persist. Case-insensitive match, applied
// recursively.
var redactKeys = map[string]struct{}{
	"password":     {},
	"access_token": {},
	"app_secret":   {},
	"verify_token": {},
	"secrets":      {},
	"plaintext":    {},
	"token":        {},
	"secret":       {},
}

// Deps bundles the state Service needs.
type Deps struct {
	// Repo persists and queries entries. Required.
	Repo repository.APITokenUsageRepo

	// Logger receives warnings when Record silently swallows a
	// persistence failure. Nil is permitted (record errors then go
	// unlogged).
	Logger *slog.Logger

	// MaxBodyBytes overrides DefaultMaxBodyBytes when > 0.
	MaxBodyBytes int
}

// Service is the application-layer entry point for api-token-usage
// bookkeeping. It has three responsibilities:
//
//  1. Record — called by the recording middleware on every bearer-auth
//     request. Must never propagate errors back to the caller.
//  2. List — served by the REST handler.
//  3. Metrics — served by the REST handler; returns aggregate KPIs +
//     per-day series + top paths + status histogram.
type Service struct {
	repo         repository.APITokenUsageRepo
	logger       *slog.Logger
	maxBodyBytes int
}

// NewService constructs a Service from the given Deps. Panics on missing
// Repo — the caller has almost certainly forgotten to wire it.
func NewService(deps Deps) *Service {
	if deps.Repo == nil {
		panic("apitokenusage.NewService: Deps.Repo is required")
	}
	max := deps.MaxBodyBytes
	if max <= 0 {
		max = DefaultMaxBodyBytes
	}
	return &Service{
		repo:         deps.Repo,
		logger:       deps.Logger,
		maxBodyBytes: max,
	}
}

// Record persists an entry. It is fire-and-forget: persistence errors
// are logged (if a logger was configured) and swallowed so a downed
// MySQL never breaks the API path.
//
// Request / response bodies are JSON-redacted (best-effort) and then
// truncated to MaxBodyBytes before persist. The domain-level *_bytes
// counters retain the true wire size regardless of truncation.
func (s *Service) Record(ctx context.Context, entry apitokenusage.Entry) {
	if s == nil {
		return
	}
	entry.RequestBody = clipBody(redactJSON(entry.RequestBody), s.maxBodyBytes)
	entry.ResponseBody = clipBody(redactJSON(entry.ResponseBody), s.maxBodyBytes)
	if err := s.repo.Record(ctx, entry); err != nil {
		if s.logger != nil {
			s.logger.Warn("apitokenusage: record failed",
				slog.String("token_id", string(entry.TokenID)),
				slog.String("org_id", string(entry.OrgID)),
				slog.String("path", entry.Path),
				slog.Any("err", err),
			)
		}
	}
}

// List returns a page of entries for org filtered by the supplied
// criteria. See repository.UsageFilter for field semantics.
func (s *Service) List(
	ctx context.Context,
	orgID organization.ID,
	filter repository.UsageFilter,
) ([]apitokenusage.Entry, string, error) {
	return s.repo.List(ctx, orgID, filter)
}

// Metrics returns aggregate KPIs, per-day series, per-status histogram,
// and top paths for one token over [from, to].
func (s *Service) Metrics(
	ctx context.Context,
	orgID organization.ID,
	tokenID apitoken.ID,
	from, to time.Time,
) (apitokenusage.Metrics, error) {
	return s.repo.Metrics(ctx, orgID, tokenID, from, to)
}

// clipBody returns b bounded at max bytes. Returns nil unchanged.
func clipBody(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	out := make([]byte, max)
	copy(out, b[:max])
	return out
}

// redactJSON returns a JSON-redacted copy of b when b parses as JSON;
// otherwise returns b unchanged. Any value whose key (case-insensitive)
// appears in redactKeys is replaced with redactedPlaceholder.
//
// Non-JSON bodies (form-encoded, binary, empty) round-trip untouched
// so the caller-visible size + shape stays honest for those payloads.
func redactJSON(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	// Cheap early-exit: only bodies that look like JSON get parsed. A
	// full unmarshal on every request would be expensive for POST
	// bodies that are, say, plain-text CSVs.
	trim := bytes.TrimLeft(b, " \t\r\n")
	if len(trim) == 0 {
		return b
	}
	first := trim[0]
	if first != '{' && first != '[' {
		return b
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return b
	}
	redacted := walkRedact(v)
	out, err := json.Marshal(redacted)
	if err != nil {
		return b
	}
	return out
}

// walkRedact walks a decoded JSON tree and overwrites any value whose
// key appears in redactKeys (case-insensitive).
func walkRedact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if _, hit := redactKeys[toLowerASCII(k)]; hit {
				out[k] = redactedPlaceholder
				continue
			}
			out[k] = walkRedact(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = walkRedact(vv)
		}
		return out
	default:
		return v
	}
}

// toLowerASCII returns s lowercased for the ASCII range. JSON keys are
// almost always ASCII so avoiding the strings.ToLower allocation +
// unicode table for every field keeps redaction cheap.
func toLowerASCII(s string) string {
	needs := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
