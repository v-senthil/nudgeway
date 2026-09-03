package providers

import "testing"

func TestRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()
	// Use a unique key so parallel tests don't collide.
	Register(Descriptor{Kind: KindChannel, Key: "test_channel", Name: "Test"})
	d, ok := Lookup(KindChannel, "test_channel")
	if !ok {
		t.Fatal("Lookup: not found")
	}
	if d.Name != "Test" {
		t.Errorf("Name = %q, want Test", d.Name)
	}
	if got := List(KindChannel); len(got) == 0 {
		t.Errorf("List returned empty")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	t.Parallel()
	Register(Descriptor{Kind: KindTicketing, Key: "dup", Name: "A"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	Register(Descriptor{Kind: KindTicketing, Key: "dup", Name: "B"})
}
