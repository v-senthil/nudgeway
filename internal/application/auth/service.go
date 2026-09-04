package auth

import (
	"context"
	"errors"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	dusr "github.com/v-senthil/nudgeway/internal/domain/user"
	infauth "github.com/v-senthil/nudgeway/internal/infrastructure/auth"
)

// UserFinder locates a user by normalized email for authentication.
type UserFinder interface {
	FindByEmail(ctx context.Context, email string) (dusr.User, error)
}

// PermissionResolver returns the permission set for (org, user).
type PermissionResolver interface {
	Resolve(ctx context.Context, orgID organization.ID, userID dusr.ID) (rbac.PermissionSet, error)
}

// Errors surfaced by the service.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotLoggable    = errors.New("user cannot log in")
)

// Service orchestrates login, logout, and "me".
type Service struct {
	users    UserFinder
	sessions infauth.SessionStore
	perms    PermissionResolver
	ttl      time.Duration
	now      func() time.Time
}

// NewService wires the auth service. `now` is injectable for tests.
func NewService(u UserFinder, s infauth.SessionStore, p PermissionResolver, ttl time.Duration) *Service {
	return &Service{users: u, sessions: s, perms: p, ttl: ttl, now: time.Now}
}

// LoginResult carries what a caller needs after a successful login.
type LoginResult struct {
	SessionID infauth.SessionID
	UserID    dusr.ID
	OrgID     organization.ID
	Perms     rbac.PermissionSet
	ExpiresAt time.Time
}

// Login authenticates by email + password and issues a session.
// It returns ErrInvalidCredentials on any failure that could reveal user
// existence — never leak the reason.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (LoginResult, error) {
	norm, err := dusr.NormalizeEmail(email)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	u, err := s.users.FindByEmail(ctx, norm)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if !u.CanLogin() {
		return LoginResult{}, ErrInvalidCredentials
	}
	ok, err := infauth.VerifyPassword(password, string(u.PasswordHash))
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	sid, err := infauth.NewSessionID()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	sess := infauth.Session{
		ID: sid, UserID: string(u.ID), OrgID: string(u.OrgID),
		IssuedAt: now, LastSeenAt: now, ExpiresAt: now.Add(s.ttl),
		IP: ip, UserAgent: ua,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return LoginResult{}, err
	}
	perms, err := s.perms.Resolve(ctx, u.OrgID, u.ID)
	if err != nil {
		perms = rbac.PermissionSet{}
	}
	return LoginResult{
		SessionID: sid, UserID: u.ID, OrgID: u.OrgID,
		Perms: perms, ExpiresAt: sess.ExpiresAt,
	}, nil
}

// Logout invalidates the session.
func (s *Service) Logout(ctx context.Context, id infauth.SessionID) error {
	return s.sessions.Delete(ctx, id)
}
