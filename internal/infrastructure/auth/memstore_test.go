package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemStore_CreateGetDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemStore()
	id, _ := NewSessionID()
	sess := Session{ID: id, UserID: "u1", OrgID: "o1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != "u1" {
		t.Errorf("UserID = %q", got.UserID)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, id); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMemStore_ExpiredTreatedAsAbsent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemStore()
	id, _ := NewSessionID()
	_ = s.Create(ctx, Session{ID: id, ExpiresAt: time.Now().Add(-time.Second)})
	if _, err := s.Get(ctx, id); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMemStore_Touch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemStore()
	id, _ := NewSessionID()
	_ = s.Create(ctx, Session{ID: id, ExpiresAt: time.Now().Add(time.Hour)})
	t0 := time.Now().Add(time.Minute)
	if err := s.Touch(ctx, id, t0); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	sess, _ := s.Get(ctx, id)
	if !sess.LastSeenAt.Equal(t0) {
		t.Errorf("LastSeenAt = %v, want %v", sess.LastSeenAt, t0)
	}
	if err := s.Touch(ctx, "nope", t0); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}
