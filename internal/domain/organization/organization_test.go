package organization

import "testing"

func TestOrganization_Active(t *testing.T) {
	t.Parallel()
	if !(Organization{Status: StatusActive}).Active() {
		t.Errorf("active org should report Active()=true")
	}
	if (Organization{Status: StatusSuspended}).Active() {
		t.Errorf("suspended org should report Active()=false")
	}
}
