package auth

import (
	"context"
	"sync"
	"time"
)

// MemStore is an in-memory SessionStore for tests and Phase 0 dev.
// Not suitable for production — replaced by the MySQL-backed store in
// Phase 0 Task 3.
type MemStore struct {
	mu sync.RWMutex
	m  map[SessionID]Session
}

// NewMemStore returns a ready MemStore.
func NewMemStore() *MemStore { return &MemStore{m: map[SessionID]Session{}} }

// Create persists a session.
func (s *MemStore) Create(_ context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sess.ID] = sess
	return nil
}

// Get returns the session or ErrSessionNotFound. Expired sessions are treated as absent.
func (s *MemStore) Get(_ context.Context, id SessionID) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.m[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if time.Now().After(sess.ExpiresAt) {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

// Touch updates last-seen and slides the expiry forward.
func (s *MemStore) Touch(_ context.Context, id SessionID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[id]
	if !ok {
		return ErrSessionNotFound
	}
	sess.LastSeenAt = at
	s.m[id] = sess
	return nil
}

// Delete removes the session.
func (s *MemStore) Delete(_ context.Context, id SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}
