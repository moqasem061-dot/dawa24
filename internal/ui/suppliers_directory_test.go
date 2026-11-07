package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
)

// The directory lists only suppliers whose profile page opens (S14).
func TestListedInDirectoryOnlyApproved(t *testing.T) {
	cases := []struct {
		status org.OrganizationStatus
		want   bool
	}{
		{org.StatusApproved, true},
		{org.StatusPending, false},
		{org.StatusRejected, false},
		{org.StatusSuspended, false},
	}
	for _, tc := range cases {
		if got := listedInDirectory(&org.Organization{Status: tc.status}); got != tc.want {
			t.Errorf("status %q listed = %v, want %v", tc.status, got, tc.want)
		}
	}
	if listedInDirectory(nil) {
		t.Error("nil organization must not be listed")
	}
}
