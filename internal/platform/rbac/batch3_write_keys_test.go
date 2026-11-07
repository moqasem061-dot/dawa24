package rbac

import "testing"

// Batch 3 (S10): admin write routes need a write key. These are the keys the
// seeded roles must keep, so that tightening the routes does not lock out a
// role that does the work.
func TestSeededRolesKeepAdminWriteKeys(t *testing.T) {
	grants := map[string]map[string]bool{}
	for _, r := range PlatformRoles() {
		set := map[string]bool{}
		for _, k := range GrantsFor(r, ScopeAdmin) {
			set[k] = true
		}
		grants[r.Key] = set
	}

	mustHave := func(role string, keys ...string) {
		t.Helper()
		for _, k := range keys {
			if !grants[role][k] {
				t.Errorf("role %s lost %s", role, k)
			}
		}
	}

	mustHave("admin",
		"catalog.match_decision.update", "catalog.match_decision.delete",
		"promo.ad.update", "promo.offer_package.update", "promo.offer_package.manage",
		"inventory.temp_warehouse.manage", "inventory.my_temp_warehouse.manage",
		"inventory.team_temp_warehouse.manage", "inventory.warehouse.manage",
	)
	mustHave("super_admin", "catalog.match_decision.update", "promo.ad.update", "promo.offer_package.update")
	mustHave("moderator",
		"inventory.temp_warehouse.manage", "inventory.my_temp_warehouse.manage",
		"inventory.team_temp_warehouse.manage",
	)

	// The support role reads ads and offers. Approving or changing them was the
	// hole S10 reported, so it must not hold the write keys.
	for _, k := range []string{"promo.ad.update", "catalog.match_decision.update", "promo.offer_package.update"} {
		if grants["support"][k] {
			t.Errorf("support role must not hold write key %s", k)
		}
	}
}
