package audit_test

import (
	"context"
	"errors"
	"testing"

	appaudit "github.com/fullwa/fullwa/internal/application/audit"
	daudit "github.com/fullwa/fullwa/internal/domain/audit"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

type stubRepo struct {
	recordErr error
	recorded  []daudit.Entry
	list      []daudit.Entry
	next      string
	listErr   error
}

func (s *stubRepo) Record(_ context.Context, e daudit.Entry) (uint64, error) {
	s.recorded = append(s.recorded, e)
	if s.recordErr != nil {
		return 0, s.recordErr
	}
	return uint64(len(s.recorded)), nil
}

func (s *stubRepo) List(_ context.Context, _ organization.ID, _ repository.AuditListFilter) ([]daudit.Entry, string, error) {
	return s.list, s.next, s.listErr
}

func TestRecord_SwallowsRepoError(t *testing.T) {
	r := &stubRepo{recordErr: errors.New("boom")}
	svc := appaudit.New(appaudit.Deps{Repo: r})
	// Must not panic and must not return an error (Record has no error).
	svc.Record(context.Background(), daudit.Entry{OrgID: "01H", Action: daudit.MessageSent})
	if len(r.recorded) != 1 {
		t.Fatalf("expected 1 recorded, got %d", len(r.recorded))
	}
}

func TestRecord_HappyPath(t *testing.T) {
	r := &stubRepo{}
	svc := appaudit.New(appaudit.Deps{Repo: r})
	svc.Record(context.Background(), daudit.Entry{
		OrgID:        "01H",
		Action:       daudit.IntegrationCreated,
		ResourceType: "integration",
		ResourceID:   "01HX",
	})
	if len(r.recorded) != 1 {
		t.Fatalf("expected 1 recorded, got %d", len(r.recorded))
	}
	if r.recorded[0].Action != daudit.IntegrationCreated {
		t.Errorf("wrong action: %v", r.recorded[0].Action)
	}
}

func TestNew_PanicsWithoutRepo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	appaudit.New(appaudit.Deps{})
}

func TestList_PassThrough(t *testing.T) {
	r := &stubRepo{
		list: []daudit.Entry{{ID: 1, Action: daudit.MessageSent}},
		next: "cursor-x",
	}
	svc := appaudit.New(appaudit.Deps{Repo: r})
	got, next, err := svc.List(context.Background(), "org", repository.AuditListFilter{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if next != "cursor-x" {
		t.Errorf("wrong cursor: %q", next)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("wrong list: %+v", got)
	}
}
