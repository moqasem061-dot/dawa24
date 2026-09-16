package rbac

// A Capability is one page or action two dashboards share, and the key each of
// them grants it under.
//
// The largest such surface is buying. A pharmacy buys; a supplier sells, and
// also buys from other suppliers, on exactly the same screens — the catalogue,
// the supplier directory, the offers board, the purchase request, the cart, the
// orders list. There is one implementation of each and there must stay one,
// because a second copy is a second set of coverage rules, a second cart and a
// second place for the self-supply refusal to be forgotten.
//
// What cannot be shared is the permission. Keys are namespaced by dashboard so
// that Catalog.Restrict is a real boundary (see catalog_vendor.go): a vendor
// owner granting their sales rep "may browse the catalogue" must grant a
// vendor. key, not a pharmacy. one, or the pharmacy scope leaks into the vendor
// role editor.
//
// A Capability is that pair, declared once. The route gate names the
// capability and resolves the caller's own key from it; each sidebar names the
// key for its own scope. Neither can drift from the other, because there is
// one declaration, and TestSharedCapabilitiesAreDeclaredInBothScopes holds it
// to the catalogue.
type Capability struct {
	// Name identifies the capability in logs and test failures. It is not a
	// permission key and is never matched against a holding.
	Name string
	// Pharmacy is the key a pharmacy role is granted this capability under.
	Pharmacy string
	// Vendor is the key a supplier role is granted it under.
	Vendor string
}

// KeyFor returns the permission key this capability is granted under on a
// dashboard, or "" for a dashboard that does not offer it.
func (c Capability) KeyFor(s Scope) string {
	switch s {
	case ScopePharmacy:
		return c.Pharmacy
	case ScopeVendor:
		return c.Vendor
	}
	return ""
}

// The buying capabilities, in the order they appear on the surface.
var (
	// BuyCatalogView opens the drug catalogue — the listing of what every
	// supplier sells.
	BuyCatalogView = Capability{"catalog.view", "pharmacy.purchase_request.view", "vendor.buying.catalog.view"}
	// BuyPurchaseRequestView opens the purchase-request gateway, which is also
	// where Smart Ordering starts.
	BuyPurchaseRequestView = Capability{"purchase_request.view", "pharmacy.purchase_request.view", "vendor.buying.purchase_request.view"}
	// BuyPurchaseRequestCreate submits a purchase request to a supplier.
	BuyPurchaseRequestCreate = Capability{"purchase_request.create", "pharmacy.purchase_request.create", "vendor.buying.purchase_request.create"}
	// BuySmartOrderView opens the Smart Ordering wizard and its history.
	BuySmartOrderView = Capability{"smart_order.view", "pharmacy.smart_order.view", "vendor.buying.smart_order.view"}
	// BuySmartOrderRun uploads a list, runs the match and places the result.
	BuySmartOrderRun = Capability{"smart_order.run", "pharmacy.smart_order.run", "vendor.buying.smart_order.run"}
	// BuyCartUse fills a basket. Separate from placing the order so a junior
	// member can prepare one for someone else to approve.
	BuyCartUse = Capability{"cart.use", "pharmacy.cart.use", "vendor.buying.cart.use"}
	// BuyOrderView opens the buyer's own orders and shipments.
	BuyOrderView = Capability{"order.view", "pharmacy.order.view", "vendor.buying.order.view"}
	// BuyOrderCreate commits the company's money.
	BuyOrderCreate = Capability{"order.create", "pharmacy.order.create", "vendor.buying.order.create"}
	// BuyOrderUpdate edits a placed order and negotiates it.
	BuyOrderUpdate = Capability{"order.update", "pharmacy.order.update", "vendor.buying.order.update"}
	// BuyOfferView opens the offers and discounts board.
	BuyOfferView = Capability{"offer.view", "pharmacy.offer.view", "vendor.buying.offer.view"}
	// BuySupplierView opens the supplier directory and one supplier's profile.
	BuySupplierView = Capability{"supplier.view", "pharmacy.supplier.view", "vendor.buying.supplier.view"}
	// BuySupplierFollow follows and messages a supplier, and opens the
	// followed list.
	BuySupplierFollow = Capability{"supplier.follow", "pharmacy.supplier.follow", "vendor.buying.supplier.follow"}
	// BuyReviewWrite rates a supplier the buyer has dealt with.
	BuyReviewWrite = Capability{"review.write", "pharmacy.review.write", "vendor.buying.review.write"}
	// BuyFavoriteView opens the saved-products list.
	BuyFavoriteView = Capability{"favorite.view", "pharmacy.favorite.view", "vendor.buying.favorite.view"}
	// BuyFavoriteManage adds and removes a saved product.
	BuyFavoriteManage = Capability{"favorite.manage", "pharmacy.favorite.manage", "vendor.buying.favorite.manage"}
)

// BuyingCapabilities is every capability on the shared surface. The audit walks
// it to prove both keys of each pair are declared and grantable.
func BuyingCapabilities() []Capability {
	return []Capability{
		BuyCatalogView, BuyPurchaseRequestView, BuyPurchaseRequestCreate,
		BuySmartOrderView, BuySmartOrderRun,
		BuyCartUse,
		BuyOrderView, BuyOrderCreate, BuyOrderUpdate,
		BuyOfferView, BuySupplierView, BuySupplierFollow, BuyReviewWrite,
		BuyFavoriteView, BuyFavoriteManage,
	}
}

// The shared company pages, outside buying.
//
// These are the routes registered in RegisterApprovedSharedRoutes: one wallet
// screen, one invoice list, one branch-manager form, reached by both dashboards
// at an unprefixed URL. They had no permission gate at all, because the audit
// that would have caught it only read files whose name contained "routes" — so
// /customer/wallet/deposit required pharmacy.wallet.manage while /wallet/deposit,
// the same handler on the same money, required nothing but membership.
var (
	// WalletView opens the company wallet.
	WalletView = Capability{"wallet.view", "pharmacy.wallet.view", "vendor.wallet.view"}
	// WalletManage deposits, withdraws and edits payment methods.
	WalletManage = Capability{"wallet.manage", "pharmacy.wallet.manage", "vendor.wallet.manage"}
	// BranchUpdate edits a branch, including who manages it.
	BranchUpdate = Capability{"branch.update", "pharmacy.branch.update", "vendor.branch.update"}
	// InvoiceView opens the company's invoices.
	//
	// A supplier's invoices are its receivables and have their own grant. A
	// pharmacy has no invoice permission of its own: its invoices are its
	// orders seen from the money side, so the order grant is the one that
	// reveals them, exactly as the pharmacy sidebar already assumes.
	InvoiceView = Capability{"invoice.view", "pharmacy.order.view", "vendor.invoice.view"}
)

// SharedCompanyCapabilities is every non-buying capability. The audit walks it
// beside BuyingCapabilities to prove both keys of each pair are declared.
func SharedCompanyCapabilities() []Capability {
	return []Capability{WalletView, WalletManage, BranchUpdate, InvoiceView}
}

// RequiredKeys returns the keys a caller on this dashboard must hold one of.
//
// It resolves by scope rather than accepting either dashboard's key. A vendor
// member can only ever be granted vendor. keys — Catalog.Restrict enforces that
// when the role is saved — so accepting both would change nothing today, and
// would quietly become a hole the day a key appears in two scopes.
//
// An unknown scope yields no keys, and a gate handed none must refuse: a caller
// with no dashboard is not a buyer.
func RequiredKeys(s Scope, caps ...Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		if k := c.KeyFor(s); k != "" {
			out = append(out, k)
		}
	}
	return out
}
