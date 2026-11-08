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
	if got := itemsOf(ScopeVendor, both); len(got) != 2 || got[0] != "/vendor/integrations" || got[1] != "/vendor/integrations/review" {
		t.Errorf("vendor section = %v", got)
	}
	if got := itemsOf(ScopeVendor, NewSet([]string{"vendor.integration.view"})); len(got) != 1 {
		t.Errorf("review shown without its permission: %v", got)
	}
	if got := itemsOf(ScopePharmacy, NewSet([]string{"pharmacy.integration.view"})); len(got) != 1 || got[0] != "/customer/integrations" {
		t.Errorf("pharmacy section = %v", got)
	}
	NavSuppressions["integrations"] = true
	t.Cleanup(func() { delete(NavSuppressions, "integrations") })
	if got := itemsOf(ScopeVendor, both); got != nil {
		t.Errorf("suppressed section still shown: %v", got)
	}
}
