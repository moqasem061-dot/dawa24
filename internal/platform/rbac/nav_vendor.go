package rbac

// vendorBuyingNav is the supplier's purchasing section — the same six screens a
// pharmacy buys through, reached at the same URLs.
//
// The nav keys are the shared buying vocabulary ("orders", "catalog",
// "offers", "suppliers", "followed", "purchase-request"), because those pages
// have one implementation and pass one activeNav value whoever is looking at
// them. The supplier's own selling screens therefore carry "supply_orders" and
// "supply_offers": one activeNav value must highlight one link, and أوامر
// التوريد (orders placed *with* this supplier) and طلباتي (orders this supplier
// placed) are two different pages that would otherwise both light up.
func vendorBuyingNav() NavSection {
	return NavSection{
		Key: "buying", NameAr: "شراء المنتجات", NameEn: "Purchasing",
		Items: []NavItem{
			{Key: "orders", Href: "/orders", Icon: "truck",
				NameAr: "طلبات والشحنات", NameEn: "Orders & shipments",
				Perm: BuyOrderView.Vendor},
			{Key: "purchase-request", Aliases: []string{"purchase_request", "smart-order", "smart_order"},
				Href: "/customer/purchase-request", Icon: "cart",
				NameAr: "طلب الشراء", NameEn: "Purchase request",
				Perm: BuyPurchaseRequestView.Vendor, Also: []string{BuySmartOrderView.Vendor}},
			{Key: "followed", Href: "/suppliers/followed", Icon: "heart-filled",
				NameAr: "الموردون المتابعون", NameEn: "Followed suppliers",
				Perm: BuySupplierFollow.Vendor},
			{Key: "suppliers", Href: "/customer/suppliers", Icon: "building",
				NameAr: "دليل الموردين", NameEn: "Supplier directory",
				Perm: BuySupplierView.Vendor},
			{Key: "catalog", Href: "/customer/catalog", Icon: "package",
				NameAr: "كتالوج الأدوية", NameEn: "Drug catalogue",
				Perm: BuyCatalogView.Vendor},
			{Key: "offers", Href: "/customer/offers", Icon: "tag",
				NameAr: "العروض والخصومات", NameEn: "Offers & discounts",
				Perm: BuyOfferView.Vendor},
		},
	}
}

// vendorDeliveryNav is إدارة الشحنات — the dispatch section.
//
// It is its own section rather than a link under الطلبات والمالية because of
// who sees it. A مندوب holds vendor.delivery.view and nothing else, so this is
// their entire sidebar; filing it under "Orders & Finance" would label a
// courier's one screen with a heading about money they never touch.
func vendorDeliveryNav() NavSection {
	return NavSection{
		Key: "delivery", NameAr: "إدارة الشحنات", NameEn: "Shipment Dispatch",
		Items: []NavItem{
			{Key: "delivery", Aliases: []string{"delivery_shipment"},
				Href: "/vendor/delivery", Icon: "truck",
				NameAr: "إدارة الشحنات", NameEn: "Shipment dispatch",
				Perm: "vendor.delivery.view"},
		},
	}
}

func vendorNav() []NavSection {
	return []NavSection{
		{
			Key: "company", NameAr: "المنشأة والفروع", NameEn: "Company & Branches",
			Items: []NavItem{
				{Key: "dashboard", Href: "/vendor/dashboard", Icon: "home",
					NameAr: "لوحة التحكم", NameEn: "Dashboard",
					Perm: "vendor.dashboard.view"},
				{Key: "organization", Href: "/vendor/organization", Icon: "building",
					NameAr: "بيانات المنشأة", NameEn: "Company profile",
					Perm: "vendor.organization.view"},
				{Key: "user_organization", Href: "/vendor/user-organization", Icon: "users",
					NameAr: "ربط المستخدمين بالمنشأة", NameEn: "User links",
					Perm: "vendor.user_org.view"},
				{Key: "branches", Href: "/vendor/branches", Icon: "layers",
					NameAr: "الفروع والمخازن", NameEn: "Branches",
					Perm: "vendor.branch.view"},
				{Key: "team", Aliases: []string{"employees"},
					Href: "/vendor/team", Icon: "users",
					NameAr: "فريق العمل والموظفون", NameEn: "Team",
					Perm: "vendor.team.view"},
				{Key: "roles", Href: "/vendor/roles", Icon: "shield",
					NameAr: "الأدوار والصلاحيات", NameEn: "Roles & permissions",
					Perm: "vendor.role.view"},
				{Key: "coverage", Href: "/vendor/coverage", Icon: "truck",
					NameAr: "التغطية الأسبوعية", NameEn: "Weekly coverage",
					Perm: "vendor.coverage.view"},
				{Key: "pharmacy_coverage", Href: "/vendor/pharmacy-coverage", Icon: "users",
					NameAr: "تغطية الصيدليات والموردين", NameEn: "Facility coverage",
					Perm: "vendor.pharmacy_coverage.view"},
				{Key: "subscription", Href: "/vendor/subscription", Icon: "sparkles",
					NameAr: "الاشتراك والعضوية", NameEn: "Subscription",
					Perm: "vendor.subscription.view"},
			},
		},
		{
			Key: "catalog", NameAr: "الكتالوج والمخزون", NameEn: "Catalog & Inventory",
			Items: []NavItem{
				{Key: "products", Aliases: []string{"variants"},
					Href: "/vendor/products", Icon: "package",
					NameAr: "أصناف المورد", NameEn: "Items",
					Perm: "vendor.product.view"},
				{Key: "ingest", Href: "/vendor/ingest", Icon: "upload",
					NameAr: "استيراد الكتالوج الذكي", NameEn: "Smart import",
					Perm: "vendor.ingest.view"},
				{Key: "saving_products", Href: "/vendor/saving-products", Icon: "tag",
					NameAr: "منتجات التوفير", NameEn: "Saving products",
					Perm: "vendor.saving_product.view"},
				{Key: "decision_memory", Href: "/vendor/decision-memory", Icon: "zap",
					NameAr: "ذاكرة قرارات المطابقة", NameEn: "Match decisions",
					Perm: "vendor.decision_memory.view"},
				{Key: "quotas", Href: "/vendor/quotas", Icon: "shield",
					NameAr: "حصص الفروع", NameEn: "Branch quotas",
					Perm: "vendor.quota.view"},
				{Key: "inventory", Href: "/vendor/inventory", Icon: "layers",
					NameAr: "إدارة المخزون", NameEn: "Inventory",
					Perm: "vendor.inventory.view"},
				{Key: "stock_alerts", Href: "/vendor/inventory/alerts", Icon: "alert-triangle",
					NameAr: "تنبيهات المخزون", NameEn: "Stock alerts",
					Perm: "vendor.inventory.view"},
				{Key: "warehouses", Href: "/vendor/warehouses", Icon: "building",
					NameAr: "المخازن ومواقع التخزين", NameEn: "Warehouses",
					Perm: "vendor.warehouse.view"},
			},
		},
		{
			Key: "promo", NameAr: "العروض والتسويق", NameEn: "Offers & Marketing",
			Items: []NavItem{
				{Key: "supply_offers", Href: "/vendor/offers", Icon: "tag",
					NameAr: "العروض والخصومات", NameEn: "Offers",
					Perm: "vendor.offer.view"},
				{Key: "offers_packages", Href: "/vendor/offers-packages", Icon: "package",
					NameAr: "باقات العروض والرعايات", NameEn: "Offer packages",
					Perm: "vendor.offer_package.view"},
				{Key: "sponsorship_requests", Href: "/vendor/sponsorship-requests", Icon: "star",
					NameAr: "طلبات الرعاية", NameEn: "Sponsorship requests",
					Perm: "vendor.offer_package.view"},
				{Key: "storefront", Href: "/vendor/storefront", Icon: "globe",
					NameAr: "الأقسام المميزة وواجهة المتجر", NameEn: "Storefront",
					Perm: "vendor.storefront.view"},
				{Key: "ads", Href: "/vendor/ads", Icon: "megaphone",
					NameAr: "الإعلانات", NameEn: "Advertisements",
					Perm: "vendor.ad.view"},
			},
		},
		{
			Key: "commerce", NameAr: "الطلبات والمالية", NameEn: "Orders & Finance",
			Items: []NavItem{
				{Key: "supply_orders", Href: "/vendor/orders", Icon: "truck",
					NameAr: "أوامر التوريد والشحنات", NameEn: "Supply orders",
					Perm: "vendor.order.view"},
				{Key: "purchase_requests", Href: "/vendor/purchase-requests", Icon: "cart",
					NameAr: "طلبات الشراء الواردة", NameEn: "Incoming purchase requests",
					Perm: "vendor.purchase_request.view"},
				{Key: "invoices", Href: "/invoices", Icon: "file",
					NameAr: "الفواتير والمستحقات", NameEn: "Invoices",
					Perm: "vendor.invoice.view"},
				{Key: "payments", Href: "/vendor/payments", Icon: "credit-card",
					NameAr: "المدفوعات", NameEn: "Payments",
					Perm: "vendor.payment.view"},
				{Key: "earnings", Href: "/vendor/earnings/order", Icon: "trending-up",
					NameAr: "الأرباح والعوائد", NameEn: "Earnings",
					Perm: "vendor.earnings.view"},
				{Key: "wallet", Href: "/vendor/wallet", Icon: "wallet",
					NameAr: "المحفظة والرصيد", NameEn: "Wallet",
					Perm: "vendor.wallet.view"},
			},
		},
		vendorDeliveryNav(),
		vendorBuyingNav(),
		{
			Key: "tools", NameAr: "الأدوات والتحليلات", NameEn: "Tools & Analytics",
			Items: []NavItem{
				{Key: "compare", Href: "/compare/tool", Icon: "trending-up",
					NameAr: "مقارنة الخصومات", NameEn: "Discount comparison",
					Perm: "vendor.compare.use"},
				{Key: "market-discounts", Href: "/market-discounts", Icon: "chart",
					NameAr: "خصومات السوق", NameEn: "Market discounts",
					Perm: "vendor.market_discounts.view"},
				{Key: "jobs", Href: "/vendor/jobs", Icon: "briefcase",
					NameAr: "وظائف التوظيف", NameEn: "Jobs",
					Perm: "vendor.job.view"},
			},
		},
		{
			Key: "content", NameAr: "المحتوى والسياسات", NameEn: "Content & Policies",
			Items: []NavItem{
				{Key: "documents", Href: "/vendor/documents", Icon: "file",
					NameAr: "المستندات والتراخيص", NameEn: "Documents",
					Perm: "vendor.document.view"},
				{Key: "policies", Href: "/vendor/policies", Icon: "shield",
					NameAr: "سياسات التوريد والضمان", NameEn: "Policies",
					Perm: "vendor.policy.view"},
				{Key: "activities", Href: "/vendor/activities", Icon: "clock",
					NameAr: "سجل العمليات والنشاط", NameEn: "Activity log",
					Perm: "vendor.activity.view", Also: []string{"vendor.team.view"}},
				{Key: "reviews", Href: "/vendor/reviews", Icon: "star",
					NameAr: "التقييمات", NameEn: "Reviews",
					Perm: "vendor.review.view"},
				{Key: "ai_logs", Href: "/vendor/ai-logs", Icon: "sparkles",
					NameAr: "سجل استهلاك الذكاء الاصطناعي", NameEn: "AI usage log",
					Perm: "vendor.ai_log.view"},
			},
		},
		{
			Key: "account", NameAr: "الأمان والجلسات", NameEn: "Security & Sessions",
			Items: []NavItem{
				{Key: "account_settings", Href: "/settings", Icon: "settings",
					NameAr: "اعدادات الحساب", NameEn: "Account settings",
					AlwaysVisible: true},
				{Key: "notifications", Href: "/notifications", Icon: "bell",
					NameAr: "مركز الإشعارات والتنبيهات", NameEn: "Notifications",
					Perm: "vendor.dashboard.view"},
				{Key: "sessions", Href: "/vendor/sessions", Icon: "shield",
					NameAr: "الأجهزة والجلسات النشطة", NameEn: "Active sessions",
					Perm: "vendor.session.view"},
				{Key: "mfa", Href: "/vendor/mfa", Icon: "lock",
					NameAr: "المصادقة الثنائية (MFA)", NameEn: "Two-Factor Auth (MFA)",
					Perm: "vendor.session.view"},
			},
		},
	}
}
