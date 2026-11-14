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

// TestNavSuppressedFuncHidesIntegrationsAtRuntime proves the runtime switch:
// the vendor and pharmacy sidebars lose the integrations section the moment the
// hook says so and get it back the moment it stops, while the staff sidebar
// keeps its entry so the admin page stays reachable.
func TestNavSuppressedFuncHidesIntegrationsAtRuntime(t *testing.T) {
	held := NewSet([]string{"vendor.integration.view", "vendor.quota.view", "pharmacy.integration.view", "platform.integration.view"})
	has := func(scope Scope) bool {
		for _, sec := range VisibleNav(scope, held) {
			for _, it := range sec.Items {
				if it.Key == "integrations" {
					return true
				}
			}
		}
		return false
	}
	off := false
	NavSuppressedFunc = func(key string) bool { return off && key == "integrations" }
	t.Cleanup(func() { NavSuppressedFunc = nil })

	for _, scope := range []Scope{ScopeVendor, ScopePharmacy, ScopeAdmin} {
		if !has(scope) {
			t.Fatalf("%s: integrations entry missing while the switch is on", scope)
		}
	}
	off = true
	for _, scope := range []Scope{ScopeVendor, ScopePharmacy} {
		if has(scope) {
			t.Errorf("%s: integrations entry still visible while the switch is off", scope)
		}
	}
	if !has(ScopeAdmin) {
		t.Error("admin: integrations entry vanished; staff must keep the page while the switch is off")
	}
	off = false
	for _, scope := range []Scope{ScopeVendor, ScopePharmacy} {
		if !has(scope) {
			t.Errorf("%s: integrations entry did not come back when the switch went on", scope)
		}
	}
}
