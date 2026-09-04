package apitoken

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	dapitoken "github.com/v-senthil/nudgeway/internal/domain/apitoken"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// Plaintext format constants.
//
//	nk_<prefix:8 base32>_<secret:40 base32>
//
// Prefix is stored plaintext (indexed, shown in the UI); secret is stored
// only as its argon2id hash and returned to the caller exactly once at
// creation time.
const (
	// PlaintextPrefix is the human-recognizable leading tag of every
	// Nudgeway API token.
	PlaintextPrefix = "nk_"
	// PrefixLen is the length of the plaintext prefix component.
	PrefixLen = 8
	// SecretLen is the length of the plaintext secret component.
	SecretLen = 40
)

// base32NoPad is Crockford-free RFC 4648 base32 without padding, matched
// to the token grammar: 32 upper-case letters + digits.
var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// PasswordHasher abstracts argon2id hash+verify so the application layer
// does not import the infrastructure package directly (dependency rule).
// Implemented by a thin adapter in cmd/server wrapping infrastructure/auth.
type PasswordHasher interface {
	// Hash returns the argon2id-encoded hash of pw.
	Hash(pw string) (string, error)
	// Verify reports whether pw matches the encoded hash.
	Verify(pw, encoded string) (bool, error)
}

// Clock abstracts time.Now so tests can inject deterministic values.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// systemClock is the default Clock backed by time.Now.
type systemClock struct{}

// Now returns time.Now in UTC.
func (systemClock) Now() time.Time { return time.Now().UTC() }

// Deps bundles the constructor arguments of Service.
type Deps struct {
	// Repo persists api_tokens rows (required).
	Repo repository.APITokenRepo
	// Hasher produces + verifies argon2id hashes (required).
	Hasher PasswordHasher
	// Clock overrides time.Now; defaults to systemClock{}.
	Clock Clock
}

// Service is the use-case entry point for API-token management.
type Service struct {
	repo   repository.APITokenRepo
	hasher PasswordHasher
	clock  Clock
}

// NewService constructs a Service wired against deps.
func NewService(deps Deps) *Service {
	if deps.Clock == nil {
		deps.Clock = systemClock{}
	}
	return &Service{repo: deps.Repo, hasher: deps.Hasher, clock: deps.Clock}
}

// PublicToken is the safe, list-view projection of a Token. It NEVER
// includes the plaintext secret or its hash.
type PublicToken struct {
	ID         dapitoken.ID
	Name       string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// Principal is the resolved identity carried by a bearer-authenticated
// request. Matches the shape of the session-cookie principal.
type Principal struct {
	OrgID   organization.ID
	UserID  user.ID
	TokenID dapitoken.ID
}

// ErrInvalidToken is returned when Verify cannot resolve a plaintext to
// an active token — bad format, unknown prefix, wrong secret, revoked,
// or expired. The concrete cause is deliberately not leaked to callers.
var ErrInvalidToken = errors.New("invalid api token")

// ErrValidation is returned when Create rejects the caller's input.
type ErrValidation struct{ Msg string }

// Error implements error.
func (e *ErrValidation) Error() string { return e.Msg }

// newValidation constructs an *ErrValidation with fmt.Sprintf semantics.
func newValidation(format string, a ...any) error {
	return &ErrValidation{Msg: fmt.Sprintf(format, a...)}
}

// Create mints a new API token for (orgID, userID). Returns the persisted
// PublicToken projection AND the one-shot plaintext string. The plaintext
// is NEVER stored — after this call returns, only the argon2id hash of the
// secret component lives on disk.
func (s *Service) Create(
	ctx context.Context,
	orgID organization.ID,
	userID user.ID,
	name string,
	expiresIn *time.Duration,
) (PublicToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PublicToken{}, "", newValidation("name required")
	}
	if l := len([]rune(name)); l > 120 {
		return PublicToken{}, "", newValidation("name too long (max 120 chars, got %d)", l)
	}
	prefix, err := randomToken(PrefixLen)
	if err != nil {
		return PublicToken{}, "", fmt.Errorf("gen prefix: %w", err)
	}
	secret, err := randomToken(SecretLen)
	if err != nil {
		return PublicToken{}, "", fmt.Errorf("gen secret: %w", err)
	}
	hash, err := s.hasher.Hash(secret)
	if err != nil {
		return PublicToken{}, "", fmt.Errorf("hash secret: %w", err)
	}
	now := s.clock.Now().UTC()
	var expiresAt *time.Time
	if expiresIn != nil && *expiresIn > 0 {
		t := now.Add(*expiresIn)
		expiresAt = &t
	}
	tok := dapitoken.Token{
		ID:         dapitoken.NewID(),
		OrgID:      orgID,
		UserID:     userID,
		Name:       name,
		Prefix:     prefix,
		SecretHash: []byte(hash),
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}
	if err := s.repo.Create(ctx, tok); err != nil {
		return PublicToken{}, "", fmt.Errorf("persist api token: %w", err)
	}
	plaintext := PlaintextPrefix + prefix + "_" + secret
	return publicOf(tok), plaintext, nil
}

// List returns every token owned by orgID, newest-first. Revoked tokens
// are included — callers surface them with a distinct badge.
func (s *Service) List(ctx context.Context, orgID organization.ID) ([]PublicToken, error) {
	rows, err := s.repo.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	out := make([]PublicToken, 0, len(rows))
	for _, t := range rows {
		out = append(out, publicOf(t))
	}
	return out, nil
}

// Revoke marks the token as revoked. Returns dapitoken.ErrNotFound when
// no row matches (orgID, id).
func (s *Service) Revoke(ctx context.Context, orgID organization.ID, id dapitoken.ID) error {
	if err := s.repo.Revoke(ctx, orgID, id, s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	return nil
}

// Verify resolves a plaintext token into a Principal. On success it
// asynchronously touches last_used_at (fire-and-forget; failures are
// swallowed) so a slow write path never blocks the caller.
//
// Returns ErrInvalidToken for every failure mode — bad format, unknown
// prefix, wrong secret, revoked, expired. The concrete cause is
// deliberately not leaked to the caller.
func (s *Service) Verify(ctx context.Context, plaintext string) (Principal, error) {
	prefix, secret, ok := parsePlaintext(plaintext)
	if !ok {
		return Principal{}, ErrInvalidToken
	}
	tok, err := s.repo.LookupByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, dapitoken.ErrNotFound) {
			return Principal{}, ErrInvalidToken
		}
		return Principal{}, fmt.Errorf("lookup api token: %w", err)
	}
	ok, err = s.hasher.Verify(secret, string(tok.SecretHash))
	if err != nil || !ok {
		return Principal{}, ErrInvalidToken
	}
	now := s.clock.Now().UTC()
	if !tok.Active(now) {
		return Principal{}, ErrInvalidToken
	}
	// Best-effort last_used_at update. Uses a detached context so we do
	// not observe the caller's cancellation once the token is already
	// resolved.
	go func(id dapitoken.ID, when time.Time) {
		bg, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.repo.TouchLastUsed(bg, id, when)
	}(tok.ID, now)
	return Principal{OrgID: tok.OrgID, UserID: tok.UserID, TokenID: tok.ID}, nil
}

// publicOf projects a Token to the safe list-view shape.
func publicOf(t dapitoken.Token) PublicToken {
	return PublicToken{
		ID:         t.ID,
		Name:       t.Name,
		Prefix:     t.Prefix,
		CreatedAt:  t.CreatedAt,
		LastUsedAt: t.LastUsedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
	}
}

// parsePlaintext splits a well-formed "nk_<prefix>_<secret>" into its
// prefix + secret components. Returns ok=false when the shape or lengths
// do not match.
func parsePlaintext(s string) (prefix, secret string, ok bool) {
	if !strings.HasPrefix(s, PlaintextPrefix) {
		return "", "", false
	}
	rest := s[len(PlaintextPrefix):]
	sep := strings.IndexByte(rest, '_')
	if sep != PrefixLen {
		return "", "", false
	}
	prefix = rest[:sep]
	secret = rest[sep+1:]
	if len(secret) != SecretLen {
		return "", "", false
	}
	return prefix, secret, true
}

// randomToken returns a base32-encoded random string of exactly n
// characters (case-insensitive alphanumeric, no padding).
func randomToken(n int) (string, error) {
	// base32 encodes 5 bytes per 8 chars.
	rawLen := (n*5 + 7) / 8
	raw := make([]byte, rawLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}
	enc := strings.ToLower(base32NoPad.EncodeToString(raw))
	if len(enc) < n {
		return "", fmt.Errorf("encode length %d < %d", len(enc), n)
	}
	return enc[:n], nil
}
