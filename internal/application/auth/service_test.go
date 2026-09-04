package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	dusr "github.com/v-senthil/nudgeway/internal/domain/user"
	infauth "github.com/v-senthil/nudgeway/internal/infrastructure/auth"
)

type stubUsers struct{ u dusr.User; err error }

func (s stubUsers) FindByEmail(_ context.Context, _ string) (dusr.User, error) {
	return s.u, s.err
}

type stubPerms struct{}

func (stubPerms) Resolve(_ context.Context, _ organization.ID, _ dusr.ID) (rbac.PermissionSet, error) {
	return rbac.NewSet(rbac.PermContactsRead), nil
}

// fastArgon uses tiny params so tests are fast.
var fastArgon = infauth.Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}

func mustHash(t *testing.T, pw string) []byte {
	t.Helper()
	h, err := infauth.HashPassword(pw, fastArgon)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return []byte(h)
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	u := dusr.User{ID: "u1", OrgID: "o1", Email: "a@b.co", PasswordHash: mustHash(t, "pw"), Status: dusr.StatusActive}
	svc := NewService(stubUsers{u: u}, infauth.NewMemStore(), stubPerms{}, time.Hour)
	res, err := svc.Login(context.Background(), "A@B.co", "pw", "1.1.1.1", "ua")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.OrgID != "o1" || !res.Perms.Has(rbac.PermContactsRead) {
		t.Errorf("result = %+v", res)
	}
	if res.SessionID == "" {
		t.Errorf("no session id")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	u := dusr.User{ID: "u1", OrgID: "o1", Email: "a@b.co", PasswordHash: mustHash(t, "pw"), Status: dusr.StatusActive}
	svc := NewService(stubUsers{u: u}, infauth.NewMemStore(), stubPerms{}, time.Hour)
	if _, err := svc.Login(context.Background(), "a@b.co", "nope", "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogin_UnknownUserYieldsSameError(t *testing.T) {
	t.Parallel()
	svc := NewService(stubUsers{err: errors.New("not found")}, infauth.NewMemStore(), stubPerms{}, time.Hour)
	if _, err := svc.Login(context.Background(), "who@ever", "pw", "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v", err)
	}
}

func TestLogin_DisabledUserRejected(t *testing.T) {
	t.Parallel()
	u := dusr.User{ID: "u1", OrgID: "o1", Email: "a@b.co", PasswordHash: mustHash(t, "pw"), Status: dusr.StatusDisabled}
	svc := NewService(stubUsers{u: u}, infauth.NewMemStore(), stubPerms{}, time.Hour)
	if _, err := svc.Login(context.Background(), "a@b.co", "pw", "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v", err)
	}
}

func TestLogout_DeletesSession(t *testing.T) {
	t.Parallel()
	store := infauth.NewMemStore()
	u := dusr.User{ID: "u1", OrgID: "o1", Email: "a@b.co", PasswordHash: mustHash(t, "pw"), Status: dusr.StatusActive}
	svc := NewService(stubUsers{u: u}, store, stubPerms{}, time.Hour)
	res, _ := svc.Login(context.Background(), "a@b.co", "pw", "", "")
	if err := svc.Logout(context.Background(), res.SessionID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := store.Get(context.Background(), res.SessionID); !errors.Is(err, infauth.ErrSessionNotFound) {
		t.Errorf("session not deleted")
	}
}
