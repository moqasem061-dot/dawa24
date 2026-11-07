package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// Batch 3 (S14): registration numbers and branch contacts reach only staff and
// signed-in users of an approved organization.
func TestCanSeeContactDetails(t *testing.T) {
	cases := []struct {
		name  string
		actor authctx.Actor
		want  bool
	}{
		{"approved pharmacy", authctx.Actor{UserID: 1, OrgStatus: string(org.StatusApproved)}, true},
		{"pending pharmacy", authctx.Actor{UserID: 1, OrgStatus: string(org.StatusPending)}, false},
		{"staff", authctx.Actor{UserID: 1, IsStaff: true}, true},
		{"no organization", authctx.Actor{UserID: 1}, false},
	}
	for _, tc := range cases {
		if got := canSeeContactDetails(tc.actor); got != tc.want {
			t.Errorf("%s: canSeeContactDetails=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHideContactDetailsBlanksOnlyContacts(t *testing.T) {
	o := &org.Organization{ID: 4, TaxNumber: "TAX-1", CommercialRegister: "CR-1"}
	branches := []*org.Branch{{ID: 1, Phone: "0100", ManagerName: "Manager", Address: "Street 1"}}

	got := hideContactDetails(o, branches)

	if o.TaxNumber != "" || o.CommercialRegister != "" {
		t.Fatalf("tax or CR still set: tax=%q cr=%q", o.TaxNumber, o.CommercialRegister)
	}
	if len(got) != 1 || got[0].Phone != "" || got[0].ManagerName != "" {
		t.Fatalf("branch contacts still set: %+v", got)
	}
	if got[0].Address != "Street 1" {
		t.Fatalf("non-contact branch field changed: %q", got[0].Address)
	}
	if branches[0].Phone != "0100" {
		t.Fatal("caller's branch slice was edited in place")
	}
}
