package rbac

// What each starter role may do, per dashboard.
//
// It lives beside roles.go rather than in it because the two answer different
// questions and change for different reasons: roles.go says which roles a
// company has and what they are called, and this says what each one holds.
// Adding a page to the supplier dashboard touches this file and not that one.

// orgRoleGrants holds the non-owner starter grants per scope. A key absent
// from the map for a scope means that role is seeded with nothing there.
var orgRoleGrants = map[Scope]map[string][]string{
	ScopeVendor: {
		"org_manager": {
			"vendor.dashboard.view", "vendor.organization.view",
			"vendor.branch.view", "vendor.branch.create", "vendor.branch.update",
			"vendor.team.view", "vendor.team.create", "vendor.team.update",
			"vendor.coverage.view", "vendor.coverage.manage",
			"vendor.pharmacy_coverage.view",
			"vendor.product.view", "vendor.product.create", "vendor.product.update",
			"vendor.quota.view", "vendor.quota.manage",
			"vendor.ingest.view", "vendor.ingest.run",
			"vendor.saving_product.view", "vendor.saving_product.manage",
			"vendor.inventory.view", "vendor.inventory.adjust",
			"vendor.warehouse.view", "vendor.warehouse.manage",
			"vendor.offer.view", "vendor.offer.manage",
			"vendor.offer_package.view", "vendor.offer_package.manage",
			"vendor.ad.view", "vendor.ad.manage",
			"vendor.storefront.view", "vendor.storefront.manage",
			"vendor.order.view", "vendor.order.update", "vendor.order.negotiate",
			"vendor.delivery.view", "vendor.delivery.assign",
			"vendor.purchase_request.view", "vendor.purchase_request.respond",
			"vendor.invoice.view", "vendor.activity.view",
			"vendor.document.view", "vendor.policy.view",
			"vendor.review.view", "vendor.review.reply",
			"vendor.wallet.view", "vendor.wallet.manage",
			"vendor.job.view", "vendor.job.manage", "vendor.session.view",
			"vendor.decision_memory.view", "vendor.decision_memory.delete",
			// Restocking from other distributors. The manager is the only
			// starter role that may spend on it; the roles below see what was
			// bought without being able to buy.
			"vendor.buying.catalog.view",
			"vendor.buying.purchase_request.view", "vendor.buying.purchase_request.create",
			"vendor.buying.smart_order.view", "vendor.buying.smart_order.run",
			"vendor.buying.cart.use",
			"vendor.buying.order.view", "vendor.buying.order.create", "vendor.buying.order.update",
			"vendor.buying.offer.view",
			"vendor.buying.supplier.view", "vendor.buying.supplier.follow",
			"vendor.buying.favorite.view", "vendor.buying.favorite.manage",
		},
		"org_accountant": {
			"vendor.dashboard.view",
			"vendor.invoice.view", "vendor.payment.view", "vendor.earnings.view",
			"vendor.wallet.view", "vendor.wallet.manage",
			"vendor.order.view", "vendor.subscription.view", "vendor.session.view",
			"vendor.buying.order.view",
		},
		"org_warehouse": {
			"vendor.dashboard.view",
			"vendor.product.view", "vendor.product.update",
			"vendor.ingest.view", "vendor.ingest.run",
			"vendor.inventory.view", "vendor.inventory.adjust",
			"vendor.warehouse.view", "vendor.warehouse.manage",
			"vendor.order.view", "vendor.order.update", "vendor.session.view",
			"vendor.delivery.view", "vendor.delivery.assign",
			"vendor.buying.catalog.view", "vendor.buying.order.view",
		},
		"org_sales_rep": {
			"vendor.dashboard.view",
			"vendor.order.view", "vendor.order.update", "vendor.order.negotiate",
			"vendor.purchase_request.view", "vendor.purchase_request.respond",
			"vendor.offer.view", "vendor.offer.manage",
			"vendor.product.view", "vendor.pharmacy_coverage.view",
			"vendor.market_discounts.view", "vendor.compare.use",
			"vendor.review.view", "vendor.review.reply", "vendor.session.view",
		},
		"org_pharmacist": {
			"vendor.dashboard.view", "vendor.product.view",
			"vendor.order.view", "vendor.document.view", "vendor.session.view",
			"vendor.buying.catalog.view", "vendor.buying.order.view",
		},
		"org_employee": {"vendor.dashboard.view", "vendor.order.view", "vendor.session.view"},
		// A مندوب gets their queue and the actions that close a parcel, and
		// deliberately not vendor.dashboard.view: the supplier dashboard
		// reports the company's sales, and a courier has no business reading
		// it. Their landing page is إدارة الشحنات.
		"org_courier": {
			"vendor.delivery.view", "vendor.delivery.update",
			"vendor.session.view",
		},
	},
	ScopePharmacy: {
		"org_manager": {
			"pharmacy.dashboard.view", "pharmacy.organization.view",
			"pharmacy.branch.view", "pharmacy.branch.create", "pharmacy.branch.update",
			"pharmacy.team.view", "pharmacy.team.create", "pharmacy.team.update",
			"pharmacy.purchase_request.view", "pharmacy.purchase_request.create",
			"pharmacy.smart_order.view", "pharmacy.smart_order.run",
			"pharmacy.order.view", "pharmacy.order.create", "pharmacy.order.update",
			"pharmacy.cart.use", "pharmacy.favorite.view", "pharmacy.favorite.manage",
			"pharmacy.offer.view", "pharmacy.saving_product.view", "pharmacy.saving_product.manage",
			"pharmacy.decision_memory.view", "pharmacy.decision_memory.delete", "pharmacy.supplier.view", "pharmacy.supplier.follow",
			"pharmacy.document.view",
			"pharmacy.wallet.view", "pharmacy.wallet.manage",
			"pharmacy.job.view", "pharmacy.job.manage", "pharmacy.session.view",
		},
		"org_accountant": {
			"pharmacy.dashboard.view", "pharmacy.order.view",
			"pharmacy.wallet.view", "pharmacy.wallet.manage",
			"pharmacy.subscription.view", "pharmacy.session.view",
		},
		"org_warehouse": {
			"pharmacy.dashboard.view", "pharmacy.order.view",
			"pharmacy.saving_product.view", "pharmacy.saving_product.manage",
			"pharmacy.smart_order.view", "pharmacy.session.view",
		},
		"org_sales_rep": {
			"pharmacy.dashboard.view", "pharmacy.order.view",
			"pharmacy.supplier.view", "pharmacy.offer.view", "pharmacy.session.view",
		},
		"org_pharmacist": {
			"pharmacy.dashboard.view",
			"pharmacy.purchase_request.view", "pharmacy.purchase_request.create",
			"pharmacy.smart_order.view", "pharmacy.smart_order.run",
			"pharmacy.order.view", "pharmacy.order.create", "pharmacy.order.update",
			"pharmacy.cart.use", "pharmacy.offer.view", "pharmacy.supplier.view",
			"pharmacy.saving_product.view", "pharmacy.favorite.view", "pharmacy.favorite.manage",
			"pharmacy.wallet.view", "pharmacy.wallet.manage",
			"pharmacy.session.view",
		},
		"org_employee": {"pharmacy.dashboard.view", "pharmacy.order.view", "pharmacy.session.view"},
	},
}
