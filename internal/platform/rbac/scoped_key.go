package rbac

import "strings"

// ScopedKey returns the key that grants, on dashboard s, what key grants on the
// dashboard it was written for.
//
// Notifications are written for an audience ("the people who may see this
// company's purchases") but name that audience with one dashboard's key. A
// supplier also buys, on the shared buying surface, and its members hold
// vendor.buying.order.view — never pharmacy.order.view, which Catalog.Restrict
// forbids in a vendor role. Filtering a supplier's purchase notifications by
// the pharmacy key therefore reached nobody but the owner, and the Telegram
// outbox dropped even the buyer's own copy as permission_revoked.
//
// The translation, in order:
//  1. a key already grantable on s is returned unchanged;
//  2. a shared Capability pairs the two dashboards' keys explicitly — the pair
//     whose name is the key's own suffix wins, so pharmacy.purchase_request.view
//     maps to the purchase-request key rather than the catalogue key that shares
//     it on the pharmacy side;
//  3. the same resource and action on the other dashboard, when it is declared
//     there (pharmacy.organization.view ↔ vendor.organization.view).
//
// Anything else — an admin key, an unknown key, a resource one dashboard does
// not have — is returned unchanged, which keeps its current meaning: only the
// owner and platform staff pass.
func (c *Catalog) ScopedKey(key string, s Scope) string {
	if key == "" || (s != ScopeVendor && s != ScopePharmacy) {
		return key
	}
	if p, ok := c.Lookup(key); ok && p.InScope(s) {
		return key
	}

	fallback := ""
	for _, cp := range append(BuyingCapabilities(), SharedCompanyCapabilities()...) {
		if cp.Pharmacy != key && cp.Vendor != key {
			continue
		}
		k := cp.KeyFor(s)
		if k == "" {
			continue
		}
		if strings.HasSuffix(key, "."+cp.Name) {
			return k
		}
		if fallback == "" {
			fallback = k
		}
	}
	if fallback != "" {
		return fallback
	}

	swapped := ""
	switch {
	case s == ScopeVendor && strings.HasPrefix(key, "pharmacy."):
		swapped = "vendor." + strings.TrimPrefix(key, "pharmacy.")
	case s == ScopePharmacy && strings.HasPrefix(key, "vendor.") && !strings.HasPrefix(key, "vendor.buying."):
		swapped = "pharmacy." + strings.TrimPrefix(key, "vendor.")
	}
	if swapped != "" {
		if p, ok := c.Lookup(swapped); ok && p.InScope(s) {
			return swapped
		}
	}
	return key
}
