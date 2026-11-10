package rbac

import "testing"

// The ERP pages sit in their own sidebar section: the keys page, then the
// review screen (supplier only), each shown by its own permission, and the
// whole section hidden when the deployment suppresses "integrations".
func TestIntegrationsNavSection(t *testing.T) {
	itemsOf := func(scope Scope, held Set) []string {
		for _, sec := range VisibleNav(scope, held) {
			if sec.Key == "integrations" {
				keys := make([]string, 0, len(sec.Items))
				for _, it := range sec.Items {
					keys = append(keys, it.Href)
				}
				return keys
			}
		}
		return nil
	}
	both := NewSet([]string{"vendor.integration.view", "vendor.integration.review"})
	want := []string{"/vendor/integrations", "/vendor/integrations/activity", "/vendor/integrations/review"}
	if got := itemsOf(ScopeVendor, both); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("vendor section = %v", got)
	}
	if got := itemsOf(ScopeVendor, NewSet([]string{"vendor.integration.view"})); len(got) != 2 {
		t.Errorf("review shown without its permission: %v", got)
	}
	pharmacy := itemsOf(ScopePharmacy, NewSet([]string{"pharmacy.integration.view"}))
	if len(pharmacy) != 2 || pharmacy[0] != "/customer/integrations" || pharmacy[1] != "/customer/integrations/activity" {
		t.Errorf("pharmacy section = %v", pharmacy)
	}
	NavSuppressions["integrations"] = true
	t.Cleanup(func() { delete(NavSuppressions, "integrations") })
	if got := itemsOf(ScopeVendor, both); got != nil {
		t.Errorf("suppressed section still shown: %v", got)
	}
}
