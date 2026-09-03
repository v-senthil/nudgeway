package user

import (
	"errors"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		out  string
		fail bool
	}{
		{"Foo@Example.com", "foo@example.com", false},
		{"  BAR@X.io ", "bar@x.io", false},
		{"", "", true},
		{"noat", "", true},
		{"space in@address", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeEmail(c.in)
		if c.fail {
			if !errors.Is(err, ErrEmailInvalid) {
				t.Errorf("NormalizeEmail(%q) err = %v, want ErrEmailInvalid", c.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeEmail(%q) unexpected err %v", c.in, err)
		}
		if got != c.out {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestUser_CanLogin(t *testing.T) {
	t.Parallel()
	if !(User{Status: StatusActive}).CanLogin() {
		t.Errorf("active user should CanLogin")
	}
	for _, s := range []Status{StatusInvited, StatusDisabled} {
		if (User{Status: s}).CanLogin() {
			t.Errorf("status %q should not CanLogin", s)
		}
	}
}
