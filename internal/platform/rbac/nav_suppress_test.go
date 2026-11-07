package rbac

import (
	"testing"
)

// TestNavSuppressionsHideIntegrationsWhenERPIsDisabled proves the deployment
// switch: when the composition root suppresses "integrations" the entry and
// its section disappear from every audience's sidebar, whatever permissions
// the caller holds, and everything else survives untouched.
func TestNavSuppressionsHideIntegrationsWhenERPIsDisabled(t *testing.T) {
	held := NewSet([]string{"vendor.integration.view", "vendor.integration.manage", "vendor.quota.view", "pharmacy.integration.view", "platform.integration.view", "platform.integration.revoke"})
	containsIntegrations := func(sections []NavSection) bool {
		for _, sec := range sections {
			for _, it := range sec.Items {
				if it.Key == "integrations" {
					return true
				}
			}
		}
		return false
	}
	countItems := func(sections []NavSection) int {
		n := 0
		for _, sec := range sections {
			n += len(sec.Items)
		}
		return n
	}
	if countItems(VisibleNav(ScopeVendor, held)) == 0 {
		t.Fatal("vendor nav is empty before suppression; the test fixture is wrong")
	}
	for _, scope := range []Scope{ScopeVendor, ScopePharmacy, ScopeAdmin} {
		if !containsIntegrations(VisibleNav(scope, held)) {
			t.Errorf("%s: integrations entry missing before suppression", scope)
		}
	}
	NavSuppressions["integrations"] = true
	t.Cleanup(func() { delete(NavSuppressions, "integrations") })
	for _, scope := range []Scope{ScopeVendor, ScopePharmacy, ScopeAdmin} {
		if containsIntegrations(VisibleNav(scope, held)) {
			t.Errorf("%s: integrations entry still visible while suppressed", scope)
		}
	}
	// The other entries survive: the suppression is per key, not a blanket.
	found := false
	for _, sec := range VisibleNav(ScopeVendor, held) {
		for _, it := range sec.Items {
			if it.Key == "quotas" {
				found = true
			}
		}
	}
	if !found {
		t.Error("vendor quotas entry vanished for no reason")
	}
}
