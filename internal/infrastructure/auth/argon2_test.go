package auth

import "testing"

func TestHashVerify_RoundTrip(t *testing.T) {
	t.Parallel()
	// Use small params so the test is fast.
	p := Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}
	h, err := HashPassword("correct horse battery staple", p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Errorf("VerifyPassword returned false for correct password")
	}
	ok, err = VerifyPassword("wrong", h)
	if err != nil {
		t.Fatalf("VerifyPassword wrong: %v", err)
	}
	if ok {
		t.Errorf("VerifyPassword returned true for wrong password")
	}
}

func TestHash_EmptyPasswordRejected(t *testing.T) {
	t.Parallel()
	if _, err := HashPassword("", DefaultArgon2Params()); err == nil {
		t.Errorf("expected error for empty password")
	}
}

func TestVerify_BadEncoding(t *testing.T) {
	t.Parallel()
	cases := []string{
		"", "not-a-hash",
		"$argon2id$v=99$m=1,t=1,p=1$YWFhYQ$YmJi",
		"$argon2id$v=19$m=BAD$YWFhYQ$YmJi",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}
