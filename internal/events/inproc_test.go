package events

import (
	"context"
	"errors"
	"testing"

	dev "github.com/v-senthil/nudgeway/internal/domain/events"
)

func TestInProc_PublishFansOut(t *testing.T) {
	t.Parallel()
	b := NewInProc()
	seen := 0
	b.Subscribe(dev.MessageReceived, func(_ dev.Envelope) error { seen++; return nil })
	b.Subscribe(dev.MessageReceived, func(_ dev.Envelope) error { seen++; return nil })
	if err := b.Publish(context.Background(), dev.Envelope{Type: dev.MessageReceived}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if seen != 2 {
		t.Errorf("handlers ran %d times, want 2", seen)
	}
}

func TestInProc_PublishCollectsFirstError(t *testing.T) {
	t.Parallel()
	b := NewInProc()
	boom := errors.New("boom")
	b.Subscribe(dev.TicketCreated, func(_ dev.Envelope) error { return boom })
	b.Subscribe(dev.TicketCreated, func(_ dev.Envelope) error { return nil })
	err := b.Publish(context.Background(), dev.Envelope{Type: dev.TicketCreated})
	if !errors.Is(err, boom) {
		t.Errorf("Publish err = %v, want boom", err)
	}
}

func TestInProc_UnknownTypeIsNoop(t *testing.T) {
	t.Parallel()
	b := NewInProc()
	if err := b.Publish(context.Background(), dev.Envelope{Type: dev.MessageSent}); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}
