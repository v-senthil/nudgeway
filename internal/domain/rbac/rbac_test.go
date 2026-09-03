package rbac

import "testing"

func TestPermissionSet_HasAdd(t *testing.T) {
	t.Parallel()
	s := NewSet(PermContactsRead)
	if !s.Has(PermContactsRead) {
		t.Errorf("Has PermContactsRead = false")
	}
	if s.Has(PermMessagesSend) {
		t.Errorf("unexpected PermMessagesSend")
	}
	s.Add(PermMessagesSend, PermUsersManage)
	if !s.Has(PermMessagesSend) || !s.Has(PermUsersManage) {
		t.Errorf("Add did not add: %v", s)
	}
}

func TestPermissionSet_NilSafe(t *testing.T) {
	t.Parallel()
	var s PermissionSet
	if s.Has(PermContactsRead) {
		t.Errorf("nil set should return false")
	}
}
