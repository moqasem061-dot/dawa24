package rbac

// The supplier dashboard, /vendor/*.
//
// These keys are namespaced "vendor." rather than reusing the module names,
// because the same module gates two different things on two dashboards:
// "catalog.product.view" means "read the platform's approved drug catalogue"
// on /admin, and a vendor owner granting it to a warehouse clerk must not
// thereby grant that. Namespacing by dashboard makes the scope check in
// Catalog.Restrict a real boundary rather than a naming convention.

const (
	gVendorCompany  = "vendor.company"
	gVendorCatalog  = "vendor.catalog"
	gVendorPromo    = "vendor.promo"
	gVendorCommerce = "vendor.commerce"
	gVendorDelivery = "vendor.delivery"
	gVendorBuying   = "vendor.buying"
	gVendorTools    = "vendor.tools"
	gVendorContent  = "vendor.content"
	gVendorAccount  = "vendor.account"
)

func vendorGroups() []Group {
	s := []Scope{ScopeVendor}
	return []Group{
		{Key: gVendorCompany, NameAr: "المنشأة والفروع", NameEn: "Company & Branches", Scopes: s, Order: 110},
		{Key: gVendorCatalog, NameAr: "الكتالوج والمخزون", NameEn: "Catalog & Inventory", Scopes: s, Order: 120},
		{Key: gVendorPromo, NameAr: "العروض والتسويق", NameEn: "Offers & Marketing", Scopes: s, Order: 130},
		{Key: gVendorCommerce, NameAr: "الطلبات والمالية", NameEn: "Orders & Finance", Scopes: s, Order: 140},
		{Key: gVendorDelivery, NameAr: "إدارة الشحنات", NameEn: "Shipment Dispatch", Scopes: s, Order: 142},
		{Key: gVendorBuying, NameAr: "شراء المنتجات", NameEn: "Purchasing", Scopes: s, Order: 145},
		{Key: gVendorTools, NameAr: "الأدوات والتحليلات", NameEn: "Tools & Analytics", Scopes: s, Order: 150},
		{Key: gVendorContent, NameAr: "المحتوى والسياسات", NameEn: "Content & Policies", Scopes: s, Order: 160},
		{Key: gVendorAccount, NameAr: "الحساب والأمان", NameEn: "Account & Security", Scopes: s, Order: 170},
	}
}

func vendorPage(key, group, nav, ar, en string) Permission {
	return Permission{Key: key, Group: group, Kind: KindPage, Nav: nav,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopeVendor}}
}

func vendorAct(key, group, ar, en string, implies ...string) Permission {
	return Permission{Key: key, Group: group, Kind: KindAction,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopeVendor}, Implies: implies}
}

func vendorPermissions() []Permission {
	out := make([]Permission, 0, 96)
	out = append(out, vendorCompanyPerms()...)
	out = append(out, vendorCatalogPerms()...)
	out = append(out, vendorPromoPerms()...)
	out = append(out, vendorCommercePerms()...)
	out = append(out, vendorDeliveryPerms()...)
	out = append(out, vendorBuyingPerms()...)
	out = append(out, vendorToolsPerms()...)
	out = append(out, vendorContentPerms()...)
	out = append(out, vendorAccountPerms()...)
	return out
}

func vendorCompanyPerms() []Permission {
	g := gVendorCompany
	return []Permission{
		vendorPage("vendor.dashboard.view", g, "dashboard", "لوحة التحكم", "Dashboard"),

		vendorPage("vendor.organization.view", g, "organization", "بيانات المنشأة", "Company profile"),
		vendorAct("vendor.organization.update", g, "تعديل بيانات المنشأة", "Edit company profile", "vendor.organization.view"),

		vendorPage("vendor.user_org.view", g, "user_organization", "ربط المستخدمين بالمنشأة", "User–organization links"),
		vendorAct("vendor.user_org.manage", g, "إدارة ربط المستخدمين", "Manage user links", "vendor.user_org.view"),

		vendorPage("vendor.branch.view", g, "branches", "الفروع والمخازن", "Branches"),
		vendorAct("vendor.branch.create", g, "إضافة فرع", "Create branch", "vendor.branch.view"),
		vendorAct("vendor.branch.update", g, "تعديل فرع", "Edit branch", "vendor.branch.view"),
		vendorAct("vendor.branch.delete", g, "حذف فرع", "Delete branch", "vendor.branch.view"),

		vendorPage("vendor.team.view", g, "team", "فريق العمل والموظفون", "Team & employees"),
		vendorAct("vendor.team.create", g, "إضافة موظف", "Add employee", "vendor.team.view"),
		vendorAct("vendor.team.update", g, "تعديل موظف", "Edit employee", "vendor.team.view"),
		vendorAct("vendor.team.delete", g, "حذف موظف", "Remove employee", "vendor.team.view"),

		vendorPage("vendor.role.view", g, "roles", "الأدوار والصلاحيات", "Roles & permissions"),
		vendorAct("vendor.role.create", g, "إنشاء دور", "Create role", "vendor.role.view"),
		vendorAct("vendor.role.update", g, "تعديل دور وصلاحياته", "Edit role and permissions", "vendor.role.view"),
		vendorAct("vendor.role.delete", g, "حذف دور", "Delete role", "vendor.role.view"),
		vendorAct("vendor.role.assign", g, "إسناد الأدوار للموظفين", "Assign roles to employees",
			"vendor.role.view", "vendor.team.view"),

		vendorPage("vendor.coverage.view", g, "coverage", "التغطية الأسبوعية", "Weekly coverage"),
		vendorAct("vendor.coverage.manage", g, "إدارة التغطية ونطاقات التوصيل", "Manage coverage and delivery bands", "vendor.coverage.view"),
		vendorPage("vendor.pharmacy_coverage.view", g, "pharmacy_coverage", "تغطية الصيدليات", "Pharmacy coverage"),

		vendorPage("vendor.subscription.view", g, "subscription", "الاشتراك والعضوية", "Subscription"),
		vendorAct("vendor.subscription.manage", g, "ترقية أو تجديد الاشتراك", "Upgrade or renew subscription", "vendor.subscription.view"),
	}
}

func vendorCatalogPerms() []Permission {
	g := gVendorCatalog
	return []Permission{
		vendorPage("vendor.product.view", g, "products", "أصناف المورد", "Vendor items"),
		vendorAct("vendor.product.create", g, "إضافة صنف", "Create item", "vendor.product.view"),
		vendorAct("vendor.product.update", g, "تعديل صنف", "Edit item", "vendor.product.view"),
		vendorAct("vendor.product.delete", g, "حذف صنف", "Delete item", "vendor.product.view"),

		vendorPage("vendor.ingest.view", g, "ingest", "استيراد الكتالوج الذكي", "Smart catalog import"),
		vendorAct("vendor.ingest.run", g, "رفع وتنفيذ الاستيراد", "Upload and commit an import", "vendor.ingest.view"),

		vendorPage("vendor.saving_product.view", g, "saving_products", "منتجات التوفير", "Saving products"),
		vendorAct("vendor.saving_product.manage", g, "إدارة واستيراد منتجات التوفير", "Manage and import saving products", "vendor.saving_product.view"),

		vendorPage("vendor.decision_memory.view", g, "decision_memory", "ذاكرة قرارات المطابقة", "Match decision memory"),
		vendorAct("vendor.decision_memory.delete", g, "مسح قرارات المطابقة", "Clear match decisions", "vendor.decision_memory.view"),

		vendorPage("vendor.quota.view", g, "quotas", "حصص الفروع", "Branch quotas"),
		vendorAct("vendor.quota.manage", g, "تعديل وتحرير حصص الفروع", "Set and release branch quotas", "vendor.quota.view"),

		vendorPage("vendor.inventory.view", g, "inventory", "إدارة المخزون", "Inventory"),
		vendorAct("vendor.inventory.adjust", g, "تسوية أرصدة المخزون", "Adjust stock", "vendor.inventory.view"),

		vendorPage("vendor.warehouse.view", g, "warehouses", "المخازن ومواقع التخزين", "Warehouses"),
		vendorAct("vendor.warehouse.manage", g, "إضافة وتعديل المخازن", "Create and edit warehouses", "vendor.warehouse.view"),
	}
}

func vendorPromoPerms() []Permission {
	g := gVendorPromo
	return []Permission{
		vendorPage("vendor.offer.view", g, "supply_offers", "العروض والخصومات", "Offers & discounts"),
		vendorAct("vendor.offer.manage", g, "إنشاء وتعديل وحذف العروض", "Create, edit and delete offers", "vendor.offer.view"),
		vendorPage("vendor.offer_package.view", g, "offers_packages", "باقات العروض والرعايات", "Offer packages & sponsorships"),
		vendorAct("vendor.offer_package.manage", g, "شراء باقات العروض", "Purchase offer packages", "vendor.offer_package.view"),
		vendorPage("vendor.storefront.view", g, "storefront", "الأقسام المميزة وواجهة المتجر", "Storefront"),
		vendorAct("vendor.storefront.manage", g, "إدارة أقسام واجهة المتجر", "Manage storefront sections", "vendor.storefront.view"),
		vendorPage("vendor.ad.view", g, "ads", "الإعلانات", "Advertisements"),
		vendorAct("vendor.ad.manage", g, "إدارة الإعلانات", "Manage advertisements", "vendor.ad.view"),
	}
}

func vendorCommercePerms() []Permission {
	g := gVendorCommerce
	return []Permission{
		vendorPage("vendor.order.view", g, "supply_orders", "أوامر التوريد والشحنات", "Supply orders"),
		vendorAct("vendor.order.update", g, "تحديث حالة الطلبات", "Update order status", "vendor.order.view"),
		vendorAct("vendor.order.negotiate", g, "قبول أو رفض التفاوض", "Accept or reject negotiation", "vendor.order.view"),

		vendorPage("vendor.purchase_request.view", g, "purchase_requests", "طلبات الشراء الواردة", "Incoming purchase requests"),
		vendorAct("vendor.purchase_request.respond", g, "الرد على طلبات الشراء", "Respond to purchase requests", "vendor.purchase_request.view"),

		vendorPage("vendor.invoice.view", g, "invoices", "الفواتير والمستحقات", "Invoices"),
		vendorPage("vendor.payment.view", g, "payments", "المدفوعات", "Payments"),
		vendorPage("vendor.earnings.view", g, "earnings", "الأرباح والعوائد", "Earnings"),

		vendorPage("vendor.wallet.view", g, "wallet", "المحفظة والرصيد", "Wallet"),
		vendorAct("vendor.wallet.manage", g, "الإيداع والسحب ووسائل الدفع", "Deposit, withdraw and payment methods", "vendor.wallet.view"),
	}
}

// vendorDeliveryPerms is إدارة الشحنات — the dispatch surface.
//
// It is the supplier's third audience, after the people who sell and the
// people who buy: the مندوب who carries a parcel to a pharmacy. Their whole
// dashboard is one page, so these keys are deliberately narrow — a courier
// granted them can see the parcels assigned to them and close them, and
// nothing else about the company.
//
// The split between .update and .assign is the split between doing the round
// and planning it. A مندوب holds .update: they move their own parcels through
// fulfilment and confirm handover with the pharmacy's PIN. A dispatcher or an
// owner holds .assign: they decide whose round a parcel belongs to. Neither
// implies the other, so a courier cannot reassign work to themselves and a
// dispatcher who never leaves the office does not appear to be carrying
// anything.
func vendorDeliveryPerms() []Permission {
	g := gVendorDelivery
	return []Permission{
		vendorPage("vendor.delivery.view", g, "delivery", "إدارة الشحنات", "Shipment dispatch"),
		vendorAct("vendor.delivery.update", g,
			"تحديث حالة الطرود المسندة وتأكيد التسليم", "Update assigned parcels and confirm handover",
			"vendor.delivery.view"),
		vendorAct("vendor.delivery.assign", g,
			"إسناد الطرود إلى المندوبين", "Assign parcels to delivery representatives",
			"vendor.delivery.view", "vendor.order.view"),
	}
}

// vendorBuyingPerms is the supplier's side of the shared buying surface.
//
// A supplier is a buyer too: it restocks from other distributors on the same
// catalogue, the same offers board and the same cart a pharmacy uses. These
// keys are the vendor half of the pairs declared in buying.go — the pharmacy
// half lives in catalog_pharmacy.go, and a route gate names the pair rather
// than either key, so the two cannot drift.
//
// They are deliberately not the vendor's selling keys. "vendor.order.view" is
// أوامر التوريد — orders other people placed with this supplier;
// "vendor.buying.order.view" is this supplier's own purchases. An owner who
// hands a warehouse clerk the first must not thereby hand them the second.
func vendorBuyingPerms() []Permission {
	g := gVendorBuying
	return []Permission{
		vendorPage(BuyCatalogView.Vendor, g, "catalog", "كتالوج الأدوية", "Drug catalogue"),

		vendorPage(BuyPurchaseRequestView.Vendor, g, "purchase-request", "طلب الشراء", "Purchase request"),
		vendorAct(BuyPurchaseRequestCreate.Vendor, g, "إنشاء طلب شراء", "Create a purchase request", BuyPurchaseRequestView.Vendor),

		vendorPage(BuySmartOrderView.Vendor, g, "smart_order", "الطلب الذكي", "Smart ordering"),
		vendorAct(BuySmartOrderRun.Vendor, g, "تشغيل الطلب الذكي وإرساله", "Run and submit a smart order", BuySmartOrderView.Vendor),

		vendorPage(BuyCartUse.Vendor, g, "cart", "سلة الشراء", "Cart"),

		vendorPage(BuyOrderView.Vendor, g, "orders", "طلباتي والشحنات", "My orders & shipments"),
		vendorAct(BuyOrderCreate.Vendor, g, "إتمام الشراء وإرسال الطلبات", "Check out and place orders", BuyOrderView.Vendor),
		vendorAct(BuyOrderUpdate.Vendor, g, "تعديل الطلبات والتفاوض", "Edit orders and negotiate", BuyOrderView.Vendor),

		vendorPage(BuyOfferView.Vendor, g, "offers", "العروض والخصومات", "Offers & discounts"),
		vendorPage(BuySupplierView.Vendor, g, "suppliers", "دليل الموردين", "Supplier directory"),
		vendorAct(BuySupplierFollow.Vendor, g, "متابعة الموردين ومراسلتهم", "Follow and message suppliers", BuySupplierView.Vendor),
		vendorAct(BuyReviewWrite.Vendor, g, "كتابة تقييمات الموردين", "Write supplier reviews", BuySupplierView.Vendor),

		vendorPage(BuyFavoriteView.Vendor, g, "favorites", "المنتجات المفضلة", "Favourites"),
		vendorAct(BuyFavoriteManage.Vendor, g, "تعديل المفضلة", "Edit favourites", BuyFavoriteView.Vendor),
	}
}

func vendorToolsPerms() []Permission {
	g := gVendorTools
	return []Permission{
		vendorPage("vendor.compare.use", g, "compare", "مقارنة الخصومات", "Discount comparison"),
		vendorPage("vendor.market_discounts.view", g, "market-discounts", "خصومات السوق", "Market discounts"),
		vendorPage("vendor.job.view", g, "jobs", "وظائف التوظيف", "Job postings"),
		vendorAct("vendor.job.manage", g, "إدارة الوظائف والمتقدمين", "Manage jobs and applicants", "vendor.job.view"),
	}
}

func vendorContentPerms() []Permission {
	g := gVendorContent
	return []Permission{
		vendorPage("vendor.document.view", g, "documents", "المستندات والتراخيص", "Documents & licences"),
		vendorAct("vendor.document.manage", g, "رفع وحذف المستندات", "Upload and delete documents", "vendor.document.view"),
		vendorPage("vendor.policy.view", g, "policies", "سياسات التوريد والضمان", "Supply policies"),
		vendorAct("vendor.policy.update", g, "تعديل السياسات", "Edit policies", "vendor.policy.view"),
		vendorPage("vendor.activity.view", g, "activities", "سجل العمليات والنشاط", "Activity log"),
		vendorPage("vendor.ai_log.view", g, "ai_logs", "سجل استهلاك الذكاء الاصطناعي", "AI consumption log"),
		vendorPage("vendor.review.view", g, "reviews", "التقييمات", "Reviews & ratings"),
		vendorAct("vendor.review.reply", g, "الرد على تقييمات الصيدليات", "Reply to pharmacy reviews", "vendor.review.view"),
	}
}

func vendorAccountPerms() []Permission {
	g := gVendorAccount
	return []Permission{
		// The Capsule assistant reads business data on the caller's behalf. It is
		// a separate grant from the screens it reads, so an owner can hand an
		// employee the dashboard without handing them a natural-language way to
		// summarise the whole company — and can withdraw it in one click.
		//
		// It never WIDENS access: every assistant tool also requires the same
		// permission the corresponding screen requires, so this grant alone
		// shows an employee nothing they could not already open.
		vendorAct("vendor.assistant.use", g,
			"استخدام الذكاء الاصطناعي (دكتور كبسولة)", "Use the Doctor Capsule AI assistant"),
		// Acting is a second, separate grant. With it, Capsule may PREPARE an
		// action this member could already take on the dashboard; the member
		// still confirms every one on a card that shows exactly what happens.
		// It never widens authority: each action also requires the permission
		// of the screen that performs it.
		vendorAct("vendor.assistant.act", g,
			"تنفيذ الإجراءات عبر دكتور كبسولة بعد التأكيد", "Let Doctor Capsule prepare actions for confirmation",
			"vendor.assistant.use"),

		vendorPage("vendor.session.view", g, "sessions", "الأجهزة والجلسات النشطة", "Active sessions"),
		vendorAct("vendor.session.revoke", g, "إنهاء الجلسات", "Revoke sessions", "vendor.session.view"),
	}
}
