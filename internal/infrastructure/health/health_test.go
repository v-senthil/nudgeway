package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReady_AllPass(t *testing.T) {
	t.Parallel()
	ok, res := Ready(context.Background(), 100*time.Millisecond, []Probe{
		{Name: "a", Check: func(context.Context) error { return nil }},
		{Name: "b", Check: func(context.Context) error { return nil }},
	})
	if !ok {
		t.Errorf("ok = false, want true")
	}
	if len(res) != 2 {
		t.Errorf("results = %d", len(res))
	}
}

func TestReady_OneFails(t *testing.T) {
	t.Parallel()
	ok, res := Ready(context.Background(), 100*time.Millisecond, []Probe{
		{Name: "a", Check: func(context.Context) error { return nil }},
		{Name: "b", Check: func(context.Context) error { return errors.New("nope") }},
	})
	if ok {
		t.Errorf("ok = true, want false")
	}
	if res[1].OK || res[1].Err == "" {
		t.Errorf("failed probe not reported: %+v", res[1])
	}
}
