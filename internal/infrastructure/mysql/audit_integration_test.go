//go:build integration

package mysql_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/fullwa/fullwa/internal/domain/audit"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/user"
	imysql "github.com/fullwa/fullwa/internal/infrastructure/mysql"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// TestAudit_RecordAndList exercises Record + List against a real MySQL
// pool. Wire-up + teardown live in the shared testhelper (added when the
// integration suite is fleshed out); the test here is scaffolding that
// documents the shape.
func TestAudit_RecordAndList(t *testing.T) {
	db := openTestDB(t) // provided by the shared integration harness
	repo := imysql.NewAudit(db)

	orgID := seedOrg(t, db)
	uid := user.ID(seedUser(t, db, orgID))

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, act := range []audit.Action{
		audit.IntegrationCreated,
		audit.MessageSent,
		audit.MessageMarkedRead,
	} {
		_, err := repo.Record(ctx, audit.Entry{
			OrgID:        organization.ID(orgID),
			ActorUserID:  &uid,
			Action:       act,
			ResourceType: "test",
			ResourceID:   "r-1",
			IP:           net.ParseIP("127.0.0.1"),
			Metadata:     map[string]any{"i": i},
			OccurredAt:   base.Add(time.Duration(i) * time.Millisecond),
		})
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	// First page, capped at 2 → cursor returned.
	entries, next, err := repo.List(ctx, organization.ID(orgID), repository.AuditListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if next == "" {
		t.Fatal("expected next cursor, got empty")
	}
	if entries[0].OccurredAt.Before(entries[1].OccurredAt) {
		t.Fatal("expected newest-first ordering")
	}

	// Second page uses the cursor.
	page2, next2, err := repo.List(ctx, organization.ID(orgID), repository.AuditListFilter{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("list p2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 entry on page 2, got %d", len(page2))
	}
	if next2 != "" {
		t.Fatal("expected exhausted cursor")
	}
}
