package rbac

// The platform staff dashboard, /admin/*.
//
// Keys already present in identity.permissions before this catalogue existed
// are reused verbatim, so the role grants already stored against them keep
// their meaning. New keys appear only where a page or action had no key at all
// — which was most of the sidebar.

const (
	gAdminOverview  = "admin.overview"
	gAdminOrgs      = "admin.organizations"
	gAdminSettings  = "admin.settings"
	gAdminCatalog   = "admin.catalog"
	gAdminCommerce  = "admin.commerce"
	gAdminPromo     = "admin.promo"
	gAdminTools     = "admin.tools"
	gAdminTrash     = "admin.trash"
	gAdminDeveloper = "admin.developer"
)

func adminGroups() []Group {
	return []Group{
		{Key: gAdminOverview, NameAr: "الرئيسية والمتابعة", NameEn: "Overview", Scopes: []Scope{ScopeAdmin}, Order: 10},
		{Key: gAdminOrgs, NameAr: "المؤسسات والمشرفون", NameEn: "Organizations & Staff", Scopes: []Scope{ScopeAdmin}, Order: 20},
		{Key: gAdminSettings, NameAr: "الإعدادات والمحتوى", NameEn: "Settings & Content", Scopes: []Scope{ScopeAdmin}, Order: 30},
		{Key: gAdminCatalog, NameAr: "المنتجات والمخزون", NameEn: "Catalog & Inventory", Scopes: []Scope{ScopeAdmin}, Order: 40},
		{Key: gAdminCommerce, NameAr: "المبيعات والمالية", NameEn: "Sales & Finance", Scopes: []Scope{ScopeAdmin}, Order: 50},
		{Key: gAdminPromo, NameAr: "العروض والتسويق", NameEn: "Offers & Marketing", Scopes: []Scope{ScopeAdmin}, Order: 60},
		{Key: gAdminTools, NameAr: "الأدوات والخدمات", NameEn: "Tools & Services", Scopes: []Scope{ScopeAdmin}, Order: 70},
		{Key: gAdminTrash, NameAr: "إدارة المحذوفات", NameEn: "Trash", Scopes: []Scope{ScopeAdmin}, Order: 80},
		{Key: gAdminDeveloper, NameAr: "المطور والتشخيص", NameEn: "Developer & Diagnostics", Scopes: []Scope{ScopeAdmin}, Order: 90},
	}
}

// adminPage declares a permission that reveals a sidebar item or a page.
func adminPage(key, group, nav, ar, en string) Permission {
	return Permission{Key: key, Group: group, Kind: KindPage, Nav: nav,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopeAdmin}}
}

// adminAct declares an action inside a page. It implies the page permission,
// so a role can never be able to change something it cannot see.
func adminAct(key, group, ar, en string, implies ...string) Permission {
	return Permission{Key: key, Group: group, Kind: KindAction,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopeAdmin}, Implies: implies}
}

func adminPermissions() []Permission {
	out := make([]Permission, 0, 128)
	out = append(out, adminOverviewPerms()...)
	out = append(out, adminOrgPerms()...)
	out = append(out, adminSettingsPerms()...)
	out = append(out, adminCatalogPerms()...)
	out = append(out, adminCommercePerms()...)
	out = append(out, adminPromoPerms()...)
	out = append(out, adminToolsPerms()...)
	out = append(out, adminTrashPerms()...)
	out = append(out, adminDeveloperPerms()...)
	return out
}

func adminOverviewPerms() []Permission {
	g := gAdminOverview
	return []Permission{
		adminPage("platform.dashboard.view", g, "dashboard", "لوحة المعلومات الرئيسية", "Main dashboard"),
		adminPage("notifications.center.view", g, "notifications", "مركز الإشعارات والتنبيهات", "Notification centre"),
		adminAct("notifications.center.send", g, "إرسال إشعار", "Send notification", "notifications.center.view"),
		// notifications.admin gates the module's JSON API group and predates
		// this catalogue; it stays a real grantable permission so a role that
		// may use the notifications page can also use its API.
		adminAct("notifications.admin", g, "واجهة الإشعارات البرمجية", "Notifications API", "notifications.center.view"),
	}
}

func adminOrgPerms() []Permission {
	g := gAdminOrgs
	return []Permission{
		adminPage("org.organization.view", g, "organizations", "المنشآت والمؤسسات", "Organizations"),
		adminAct("org.organization.update", g, "تعديل بيانات المنشأة", "Edit organization", "org.organization.view"),
		adminAct("org.organization.delete", g, "حذف منشأة", "Delete organization", "org.organization.view"),
		adminAct("org.admin", g, "واجهة المنشآت البرمجية", "Organizations API", "org.organization.view"),
		adminAct("org.profile.manage", g, "إدارة الملف التعريفي للمنشأة", "Manage organization profile", "org.organization.view"),
		adminAct("org.member.manage", g, "إدارة أعضاء المنشأة", "Manage organization members", "org.organization.view"),

		adminPage("org.branch.view", g, "branches", "الفروع والمستودعات", "Branches"),
		adminAct("org.branch.update", g, "تعديل الفروع", "Edit branches", "org.branch.view"),
		adminAct("org.branch.delete", g, "حذف الفروع", "Delete branches", "org.branch.view"),

		adminPage("identity.user.view", g, "users", "المستخدمون والحسابات", "Users & accounts"),
		adminAct("identity.user.update", g, "تعديل المستخدمين وتعليق الحسابات", "Edit and suspend users", "identity.user.view"),
		adminAct("identity.user.delete", g, "حذف المستخدمين", "Delete users", "identity.user.view"),
		adminAct("identity.admin", g, "واجهة الهوية البرمجية", "Identity API", "identity.user.view"),

		adminPage("identity.admin_role.view", g, "roles", "الأدوار والصلاحيات", "Roles & permissions"),
		adminAct("identity.admin_role.update", g, "إنشاء وتعديل الأدوار", "Create and edit roles", "identity.admin_role.view"),
		adminAct("identity.admin_role.delete", g, "حذف الأدوار", "Delete roles", "identity.admin_role.view"),
		adminAct("identity.admin_role.assign", g, "إسناد الأدوار للمستخدمين", "Assign roles to users",
			"identity.admin_role.view", "identity.user.view"),

		adminPage("org.approval.view", g, "approvals", "اعتماد المنشآت والوثائق", "Approvals"),
		adminAct("org.approval.decide", g, "قبول أو رفض طلبات الاعتماد", "Approve or reject requests", "org.approval.view"),
		adminPage("hr.document.view", g, "approvals", "وثائق المنشآت", "Organization documents"),
		adminAct("hr.document.update", g, "مراجعة الوثائق", "Review documents", "hr.document.view"),

		adminPage("identity.activity.view", g, "employee_activities", "نشاط الموظفين", "Employee activity"),
		adminPage("platform.chat.view", g, "chat_history", "سجل المحادثات", "Chat history"),

		adminPage("org.review.view", g, "organizations", "تقييمات المنشآت", "Organization reviews"),
		adminAct("org.review.update", g, "تعديل التقييمات", "Edit reviews", "org.review.view"),
		adminAct("org.review.delete", g, "حذف التقييمات", "Delete reviews", "org.review.view"),
	}
}

func adminSettingsPerms() []Permission {
	g := gAdminSettings
	return []Permission{
		adminPage("platform.setting.view", g, "settings", "إعدادات النظام العامة", "System settings"),
		adminAct("platform.setting.update", g, "تعديل إعدادات النظام", "Edit system settings", "platform.setting.view"),
		adminAct("platform.settings.manage", g, "إدارة الملفات والوسائط", "Manage files and media", "platform.setting.view"),

		adminPage("org.institutional_work.view", g, "institutional", "الأنشطة والأعمال المؤسسية", "Institutional activities"),
		adminAct("org.institutional_work.update", g, "تعديل الأنشطة المؤسسية", "Edit institutional activities", "org.institutional_work.view"),
		adminAct("org.institutional_work.delete", g, "حذف الأنشطة المؤسسية", "Delete institutional activities", "org.institutional_work.view"),

		adminPage("platform.geo.view", g, "cities", "المحافظات والمدن", "Governorates & cities"),
		adminAct("platform.geo.update", g, "تعديل المحافظات والمدن", "Edit governorates and cities", "platform.geo.view"),

		adminPage("platform.content.view", g, "content", "محتوى الصفحات والأقسام", "Page content"),
		adminAct("platform.content.update", g, "تعديل المحتوى", "Edit content", "platform.content.view"),
		adminAct("platform.content.delete", g, "حذف المحتوى", "Delete content", "platform.content.view"),

		adminPage("platform.translation.view", g, "translations", "اللغات والترجمة", "Languages & translation"),
		adminAct("platform.translation.update", g, "تعديل الترجمات", "Edit translations", "platform.translation.view"),

		adminPage("platform.page_control.view", g, "system_pages", "التحكم في صفحات النظام", "System pages"),
		adminAct("platform.page_control.create", g, "إضافة صفحة نظام", "Add system page", "platform.page_control.view"),
		adminAct("platform.page_control.update", g, "تفعيل/تعطيل صفحات النظام", "Toggle system pages", "platform.page_control.view"),
		adminAct("platform.page_control.delete", g, "حذف صفحة نظام مخصّصة", "Delete custom system page", "platform.page_control.view"),
	}
}

func adminCatalogPerms() []Permission {
	g := gAdminCatalog
	return []Permission{
		adminPage("catalog.product.view", g, "products", "كتالوج الأدوية المعتمدة", "Approved drug catalog"),
		adminAct("catalog.product.create", g, "إضافة منتج", "Create product", "catalog.product.view"),
		adminAct("catalog.product.update", g, "تعديل منتج", "Edit product", "catalog.product.view"),
		adminAct("catalog.product.delete", g, "حذف منتج", "Delete product", "catalog.product.view"),
		adminAct("catalog.admin", g, "واجهة الكتالوج البرمجية", "Catalog API", "catalog.product.view"),

		adminPage("catalog.category.view", g, "categories", "فئات المنتجات", "Product categories"),
		adminAct("catalog.category.update", g, "تعديل الفئات", "Edit categories", "catalog.category.view"),
		adminAct("catalog.category.delete", g, "حذف الفئات", "Delete categories", "catalog.category.view"),
		adminAct("catalog.category.manage", g, "إدارة شجرة الفئات", "Manage category tree", "catalog.category.view"),

		adminPage("catalog.import.view", g, "import", "استيراد الكتالوج العام", "Catalog import"),
		adminAct("catalog.import.run", g, "تشغيل عملية الاستيراد", "Run an import", "catalog.import.view"),
		adminPage("catalog.org_import.view", g, "org_imports", "استيراد المنتجات لمنظمات", "Organization product import"),
		adminAct("catalog.org_import.run", g, "تشغيل استيراد المنتجات للمنظمات", "Run organization import", "catalog.org_import.view"),
		adminPage("catalog.image_import.view", g, "product_images_import", "استيراد صور المنتجات", "Product image import"),
		adminAct("catalog.image_import.run", g, "تشغيل استيراد الصور", "Run image import", "catalog.image_import.view"),

		adminPage("catalog.brand.view", g, "brands", "الشركات المصنعة", "Manufacturers"),
		adminAct("catalog.brand.update", g, "تعديل الشركات المصنعة", "Edit manufacturers", "catalog.brand.view"),
		adminAct("catalog.brand.delete", g, "حذف الشركات المصنعة", "Delete manufacturers", "catalog.brand.view"),
		adminAct("catalog.brand.manage", g, "إدارة الشركات المصنعة", "Manage manufacturers", "catalog.brand.view"),

		adminPage("catalog.vendor_product.view", g, "product_child", "أصناف الموردين", "Vendor items"),
		adminAct("catalog.vendor_product.update", g, "تعديل أصناف الموردين", "Edit vendor items", "catalog.vendor_product.view"),

		adminPage("catalog.saving_product.view", g, "saving_products", "منتجات التوفير", "Saving products"),
		adminAct("catalog.saving_product.update", g, "تعديل منتجات التوفير", "Edit saving products", "catalog.saving_product.view"),

		adminPage("catalog.match_decision.view", g, "match_decisions", "ذاكرة قرارات المطابقة", "Match decision memory"),
		adminAct("catalog.match_decision.delete", g, "مسح قرارات المطابقة", "Clear match decisions", "catalog.match_decision.view"),

		adminPage("inventory.warehouse.view", g, "warehouses", "المخازن", "Warehouses"),
		adminAct("inventory.warehouse.update", g, "تعديل المخازن", "Edit warehouses", "inventory.warehouse.view"),
		adminAct("inventory.warehouse.delete", g, "حذف المخازن", "Delete warehouses", "inventory.warehouse.view"),
		adminAct("inventory.warehouse.manage", g, "إدارة المخازن", "Manage warehouses", "inventory.warehouse.view"),
		adminAct("inventory.admin", g, "واجهة المخزون البرمجية", "Inventory API", "inventory.warehouse.view"),

		adminPage("inventory.temp_warehouse.view", g, "temp_warehouses", "المستودعات المؤقتة", "Temporary warehouses"),
		adminPage("inventory.my_temp_warehouse.view", g, "my_temp_warehouses", "مستودعاتي المرفوعة", "My uploaded warehouses"),
		adminAct("inventory.my_temp_warehouse.manage", g, "إدارة مستودعاتي المرفوعة", "Manage my uploaded warehouses", "inventory.my_temp_warehouse.view"),

		// A main moderator's view of the moderators reporting to them. It is a
		// permission of its own rather than a widening of
		// inventory.temp_warehouse.view, because the two answer different
		// questions: that one is "every warehouse on the platform", this one is
		// "the warehouses of the people who report to me". A holder of this
		// sees their subordinates' uploads and nothing else, and the scoping is
		// enforced in the query, not by the sidebar. See
		// internal/ui/admin_team_temp_warehouse_handlers.go.
		adminPage("inventory.team_temp_warehouse.view", g, "team_temp_warehouses",
			"مستودعات المشرفين تحت إدارتي", "Warehouses of my moderators"),
		adminAct("inventory.team_temp_warehouse.manage", g,
			"إدارة مستودعات المشرفين تحت إدارتي", "Manage my moderators' warehouses",
			"inventory.team_temp_warehouse.view"),
		adminAct("identity.moderator.assign", g,
			"تعيين المشرفين وتحديد المشرف الرئيسي", "Assign moderators and their parent",
			"identity.user.view"),
		adminPage("inventory.stock.view", g, "warehouses", "أرصدة المخزون", "Stock levels"),
		adminAct("inventory.stock.adjust", g, "تسوية المخزون", "Adjust stock", "inventory.stock.view"),
		adminPage("inventory.transfer.view", g, "warehouses", "تحويلات المخزون", "Stock transfers"),
		adminAct("inventory.transfer.create", g, "إنشاء تحويل", "Create transfer", "inventory.transfer.view"),
		adminAct("inventory.transfer.update", g, "تعديل تحويل", "Edit transfer", "inventory.transfer.view"),
		adminAct("inventory.transfer.approve", g, "اعتماد تحويل", "Approve transfer", "inventory.transfer.view"),

		adminPage("workflow.coverage.view", g, "weekly_coverages", "التغطية الأسبوعية", "Weekly coverage"),
		adminAct("workflow.coverage.update", g, "تعديل التغطية", "Edit coverage", "workflow.coverage.view"),
	}
}

func adminCommercePerms() []Permission {
	g := gAdminCommerce
	return []Permission{
		adminPage("commerce.order.view", g, "orders", "طلبات الشراء وأوامر التوريد", "Orders"),
		adminAct("commerce.order.update", g, "تعديل الطلبات", "Edit orders", "commerce.order.view"),
		adminAct("commerce.order.dispatch", g, "إرسال الطلبات للموردين", "Dispatch orders", "commerce.order.view"),
		adminAct("commerce.order.fulfil", g, "إغلاق وتنفيذ الطلبات", "Fulfil orders", "commerce.order.view"),
		adminAct("commerce.admin", g, "واجهة الطلبات البرمجية", "Commerce API", "commerce.order.view"),
		adminPage("commerce.quote.view", g, "orders", "عروض الأسعار", "Quotes"),
		adminAct("commerce.quote.manage", g, "إدارة عروض الأسعار", "Manage quotes", "commerce.quote.view"),

		adminPage("billing.finance.view", g, "finance", "الإدارة المالية والمحافظ", "Finance & wallets"),
		adminAct("billing.admin", g, "واجهة المالية البرمجية", "Billing API", "billing.finance.view"),
		adminPage("billing.invoice.view", g, "finance", "الفواتير", "Invoices"),
		adminAct("billing.invoice.read", g, "قراءة تفاصيل الفواتير", "Read invoice detail", "billing.invoice.view"),
		adminAct("billing.invoice.manage", g, "إدارة الفواتير", "Manage invoices", "billing.invoice.view"),
		adminPage("billing.payment.view", g, "finance", "المدفوعات", "Payments"),
		adminAct("billing.payment.update", g, "تعديل المدفوعات", "Edit payments", "billing.payment.view"),
		adminAct("billing.payment.manage", g, "إدارة وسائل الدفع", "Manage payment methods", "billing.payment.view"),
		adminPage("billing.wallet.read", g, "finance", "المحافظ والأرصدة", "Wallets"),
		adminAct("billing.wallet.manage", g, "تسوية أرصدة المحافظ", "Adjust wallet balances", "billing.wallet.read"),

		adminPage("billing.subscription_plan.view", g, "plans", "باقات وخطط الاشتراك", "Subscription plans"),
		adminAct("billing.subscription_plan.update", g, "تعديل الباقات", "Edit plans", "billing.subscription_plan.view"),
		adminPage("billing.session_plan.view", g, "plans", "حدود الجلسات للباقات", "Session limits"),
		adminAct("billing.session_plan.update", g, "تعديل حدود الجلسات", "Edit session limits", "billing.session_plan.view"),
	}
}

func adminPromoPerms() []Permission {
	g := gAdminPromo
	return []Permission{
		adminPage("promo.offer.view", g, "offers", "مراجعة عروض الموردين", "Vendor offers"),
		adminAct("promo.offer.update", g, "تعديل واعتماد العروض", "Edit and approve offers", "promo.offer.view"),
		adminAct("promo.offer.manage", g, "إدارة العروض والباقات", "Manage offers and packages", "promo.offer.view"),
		adminAct("promo.admin", g, "واجهة العروض البرمجية", "Promo API", "promo.offer.view"),
		adminPage("promo.offer_package.view", g, "offers_packages", "باقات العروض والرعايات", "Offer packages"),
		adminAct("promo.offer_package.update", g, "تعديل باقات العروض", "Edit offer packages", "promo.offer_package.view"),
		adminAct("promo.offer_package.manage", g, "إدارة باقات العروض", "Manage offer packages", "promo.offer_package.view"),
		adminPage("promo.offer_location.view", g, "offer_locations", "مواقع تغطية العروض", "Offer coverage locations"),
		adminAct("promo.offer_location.update", g, "تعديل مواقع العروض", "Edit offer locations", "promo.offer_location.view"),

		adminPage("promo.ad.view", g, "ads", "الإعلانات", "Advertisements"),
		adminAct("promo.ad.update", g, "تعديل الإعلانات", "Edit advertisements", "promo.ad.view"),
		adminAct("promo.ad.delete", g, "حذف الإعلانات", "Delete advertisements", "promo.ad.view"),
		adminPage("promo.ad_plan.view", g, "ad_plans", "خطط الإعلانات", "Advertisement plans"),
		adminAct("promo.ad_plan.update", g, "تعديل خطط الإعلانات", "Edit advertisement plans", "promo.ad_plan.view"),
		adminPage("promo.adv_product.view", g, "adv_products", "رعاية المنتجات", "Product sponsorships"),
		adminAct("promo.adv_product.update", g, "تعديل رعاية المنتجات", "Edit product sponsorships", "promo.adv_product.view"),

		adminPage("platform.analytics.view", g, "analytics", "التحليلات والإحصائيات", "Analytics"),
	}
}

func adminToolsPerms() []Permission {
	g := gAdminTools
	return []Permission{
		// The Capsule assistant reads business data on the caller's behalf. It is
		// a separate grant from the screens it reads, so an owner can hand an
		// employee the dashboard without handing them a natural-language way to
		// summarise the whole company — and can withdraw it in one click.
		//
		// It never WIDENS access: every assistant tool also requires the same
		// permission the corresponding screen requires, so this grant alone
		// shows an employee nothing they could not already open.
		adminAct("platform.assistant.use", g,
			"استخدام الذكاء الاصطناعي (دكتور كبسولة)", "Use the Doctor Capsule AI assistant"),
		// Acting is a second, separate grant. With it, Capsule may PREPARE an
		// action this member could already take on the dashboard; the member
		// still confirms every one on a card that shows exactly what happens.
		// It never widens authority: each action also requires the permission
		// of the screen that performs it.
		adminAct("platform.assistant.act", g,
			"تنفيذ الإجراءات عبر دكتور كبسولة بعد التأكيد", "Let Doctor Capsule prepare actions for confirmation",
			"platform.assistant.use"),

		adminPage("platform.message.view", g, "messages", "رسائل واستفسارات التواصل", "Contact messages"),
		adminAct("platform.message.update", g, "الرد على الرسائل وإغلاقها", "Reply to and close messages", "platform.message.view"),

		adminPage("hr.job.view", g, "jobs", "وظائف وشواغر القطاع", "Job listings"),
		adminAct("hr.job.update", g, "تعديل الوظائف", "Edit jobs", "hr.job.view"),
		adminAct("hr.job.delete", g, "حذف الوظائف", "Delete jobs", "hr.job.view"),
		adminAct("hr.job.manage", g, "إدارة طلبات التوظيف", "Manage applications", "hr.job.view"),
		adminAct("hr.admin", g, "واجهة التوظيف البرمجية", "HR API", "hr.job.view"),

		adminPage("workflow.request.view", g, "requests", "طلبات المنشآت", "Organization requests"),
		adminAct("workflow.request.update", g, "الرد على الطلبات", "Respond to requests", "workflow.request.view"),
		adminPage("workflow.issue.view", g, "requests", "البلاغات والشكاوى", "Issues"),
		adminAct("workflow.issue.update", g, "معالجة البلاغات", "Handle issues", "workflow.issue.view"),
		adminAct("workflow.admin", g, "واجهة سير العمل البرمجية", "Workflow API", "workflow.issue.view"),

		adminPage("ingest.session.view", g, "import", "جلسات الاستيراد", "Import sessions"),
		adminAct("ingest.session.update", g, "تعديل جلسات الاستيراد", "Edit import sessions", "ingest.session.view"),
		adminAct("ingest.import.run", g, "تشغيل الاستيراد", "Run import", "ingest.session.view"),
		adminAct("ingest.admin", g, "واجهة الاستيراد البرمجية", "Ingest API", "ingest.session.view"),
	}
}

func adminTrashPerms() []Permission {
	g := gAdminTrash
	return []Permission{
		adminPage("platform.trash.view", g, "trash", "سلة المحذوفات الشاملة", "Trash"),
		adminAct("platform.trash.update", g, "استعادة العناصر المحذوفة", "Restore deleted items", "platform.trash.view"),
		adminAct("platform.trash.purge", g, "الحذف النهائي", "Purge permanently", "platform.trash.view"),
	}
}

func adminDeveloperPerms() []Permission {
	g := gAdminDeveloper
	return []Permission{
		adminPage("platform.developer.sql", g, "developers", "أدوات المطورين والـ AI", "Developer tools"),
		adminAct("platform.admin", g, "واجهة المنصة البرمجية", "Platform API", "platform.developer.sql"),
		adminPage("platform.error_log.view", g, "developers", "سجل الأخطاء", "Error log"),
		adminAct("platform.error_log.update", g, "تعليم الأخطاء كمعالجة", "Mark errors resolved", "platform.error_log.view"),
		adminAct("platform.error_log.delete", g, "حذف سجلات الأخطاء", "Delete error logs", "platform.error_log.view"),
		adminPage("platform.activity_log.view", g, "developers", "سجل النشاط", "Activity log"),
		adminAct("platform.activity_log.delete", g, "حذف سجل النشاط", "Delete activity log", "platform.activity_log.view"),
		adminPage("platform.ai.view", g, "ai_logs", "سجل استهلاك الذكاء الاصطناعي", "AI consumption log"),
	}
}
