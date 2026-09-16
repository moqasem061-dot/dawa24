package rbac

// The pharmacy dashboard, /customer/*.
//
// Same reasoning as the vendor catalogue: keys are namespaced by dashboard so
// that a pharmacy owner granting "buy on our behalf" to a branch pharmacist
// cannot, through a shared key, grant anything on another dashboard.

const (
	gPharmacyOrders  = "pharmacy.orders"
	gPharmacyMarket  = "pharmacy.market"
	gPharmacyCompany = "pharmacy.company"
	gPharmacyAccount = "pharmacy.account"
)

func pharmacyGroups() []Group {
	s := []Scope{ScopePharmacy}
	return []Group{
		{Key: gPharmacyOrders, NameAr: "الطلبات والشحنات", NameEn: "Orders & Shipments", Scopes: s, Order: 210},
		{Key: gPharmacyMarket, NameAr: "العروض والسوق", NameEn: "Offers & Market", Scopes: s, Order: 220},
		{Key: gPharmacyCompany, NameAr: "المنشأة والفروع", NameEn: "Company & Branches", Scopes: s, Order: 230},
		{Key: gPharmacyAccount, NameAr: "الحساب والأمان", NameEn: "Account & Security", Scopes: s, Order: 240},
	}
}

func pharmacyPage(key, group, nav, ar, en string) Permission {
	return Permission{Key: key, Group: group, Kind: KindPage, Nav: nav,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopePharmacy}}
}

func pharmacyAct(key, group, ar, en string, implies ...string) Permission {
	return Permission{Key: key, Group: group, Kind: KindAction,
		NameAr: ar, NameEn: en, Scopes: []Scope{ScopePharmacy}, Implies: implies}
}

func pharmacyPermissions() []Permission {
	out := make([]Permission, 0, 64)
	out = append(out, pharmacyOrderPerms()...)
	out = append(out, pharmacyMarketPerms()...)
	out = append(out, pharmacyCompanyPerms()...)
	out = append(out, pharmacyAccountPerms()...)
	return out
}

func pharmacyOrderPerms() []Permission {
	g := gPharmacyOrders
	return []Permission{
		pharmacyPage("pharmacy.dashboard.view", g, "dashboard", "لوحة التحكم", "Dashboard"),

		pharmacyPage("pharmacy.purchase_request.view", g, "purchase-request", "طلب الشراء", "Purchase request"),
		pharmacyAct("pharmacy.purchase_request.create", g, "إنشاء طلب شراء", "Create a purchase request", "pharmacy.purchase_request.view"),

		pharmacyPage("pharmacy.smart_order.view", g, "smart_order", "الطلب الذكي", "Smart ordering"),
		pharmacyAct("pharmacy.smart_order.run", g, "تشغيل الطلب الذكي وإرساله", "Run and submit a smart order", "pharmacy.smart_order.view"),

		pharmacyPage("pharmacy.order.view", g, "orders", "الطلبات والشحنات", "Orders & shipments"),
		pharmacyAct("pharmacy.order.create", g, "إتمام الشراء وإرسال الطلبات", "Check out and place orders", "pharmacy.order.view"),
		pharmacyAct("pharmacy.order.update", g, "تعديل الطلبات والتفاوض", "Edit orders and negotiate", "pharmacy.order.view"),
		pharmacyPage("pharmacy.cart.use", g, "cart", "سلة الشراء", "Cart"),

		pharmacyPage("pharmacy.wallet.view", g, "wallet", "المحفظة والرصيد", "Wallet"),
		pharmacyAct("pharmacy.wallet.manage", g, "الإيداع والسحب ووسائل الدفع", "Deposit, withdraw and payment methods", "pharmacy.wallet.view"),

		pharmacyPage("pharmacy.favorite.view", g, "favorites", "المنتجات المفضلة", "Favourites"),
		pharmacyAct("pharmacy.favorite.manage", g, "تعديل المفضلة", "Edit favourites", "pharmacy.favorite.view"),
	}
}

func pharmacyMarketPerms() []Permission {
	g := gPharmacyMarket
	return []Permission{
		pharmacyPage("pharmacy.offer.view", g, "offers", "العروض والخصومات", "Offers & discounts"),
		pharmacyPage("pharmacy.saving_product.view", g, "saving_products", "منتجات التوفير", "Saving products"),
		pharmacyAct("pharmacy.saving_product.manage", g, "إدارة واستيراد منتجات التوفير", "Manage and import saving products", "pharmacy.saving_product.view"),
		pharmacyPage("pharmacy.decision_memory.view", g, "decision_memory", "ذاكرة قرارات المطابقة", "Match decision memory"),
		pharmacyAct("pharmacy.decision_memory.delete", g, "مسح قرارات المطابقة", "Clear match decisions", "pharmacy.decision_memory.view"),
		pharmacyPage("pharmacy.supplier.view", g, "suppliers", "دليل الموردين", "Supplier directory"),
		pharmacyAct("pharmacy.supplier.follow", g, "متابعة الموردين ومراسلتهم", "Follow and message suppliers", "pharmacy.supplier.view"),
		pharmacyPage("pharmacy.review.write", g, "suppliers", "كتابة التقييمات", "Write reviews"),
	}
}

func pharmacyCompanyPerms() []Permission {
	g := gPharmacyCompany
	return []Permission{
		pharmacyPage("pharmacy.organization.view", g, "organization", "بيانات المنشأة", "Company profile"),
		pharmacyAct("pharmacy.organization.update", g, "تعديل بيانات المنشأة", "Edit company profile", "pharmacy.organization.view"),

		pharmacyPage("pharmacy.branch.view", g, "branches", "فروع الصيدلية", "Pharmacy branches"),
		pharmacyAct("pharmacy.branch.create", g, "إضافة فرع", "Create branch", "pharmacy.branch.view"),
		pharmacyAct("pharmacy.branch.update", g, "تعديل فرع", "Edit branch", "pharmacy.branch.view"),
		pharmacyAct("pharmacy.branch.delete", g, "حذف فرع", "Delete branch", "pharmacy.branch.view"),

		pharmacyPage("pharmacy.team.view", g, "team", "فريق العمل والموظفون", "Team & employees"),
		pharmacyAct("pharmacy.team.create", g, "إضافة موظف", "Add employee", "pharmacy.team.view"),
		pharmacyAct("pharmacy.team.update", g, "تعديل موظف", "Edit employee", "pharmacy.team.view"),
		pharmacyAct("pharmacy.team.delete", g, "حذف موظف", "Remove employee", "pharmacy.team.view"),

		pharmacyPage("pharmacy.role.view", g, "roles", "الأدوار والصلاحيات", "Roles & permissions"),
		pharmacyAct("pharmacy.role.create", g, "إنشاء دور", "Create role", "pharmacy.role.view"),
		pharmacyAct("pharmacy.role.update", g, "تعديل دور وصلاحياته", "Edit role and permissions", "pharmacy.role.view"),
		pharmacyAct("pharmacy.role.delete", g, "حذف دور", "Delete role", "pharmacy.role.view"),
		pharmacyAct("pharmacy.role.assign", g, "إسناد الأدوار للموظفين", "Assign roles to employees",
			"pharmacy.role.view", "pharmacy.team.view"),

		pharmacyPage("pharmacy.document.view", g, "documents", "المستندات والتراخيص", "Documents & licences"),
		pharmacyAct("pharmacy.document.manage", g, "رفع وحذف المستندات", "Upload and delete documents", "pharmacy.document.view"),

		pharmacyPage("pharmacy.user_org.view", g, "user_organization", "ربط المستخدمين بالمنشأة", "User–organization links"),
		pharmacyAct("pharmacy.user_org.manage", g, "إدارة ربط المستخدمين", "Manage user links", "pharmacy.user_org.view"),

		pharmacyPage("pharmacy.subscription.view", g, "subscription", "الاشتراك والعضوية", "Subscription"),
		pharmacyAct("pharmacy.subscription.manage", g, "ترقية أو تجديد الاشتراك", "Upgrade or renew subscription", "pharmacy.subscription.view"),

		pharmacyPage("pharmacy.job.view", g, "jobs", "الوظائف والتوظيف", "Jobs & recruitment"),
		pharmacyAct("pharmacy.job.manage", g, "إدارة الوظائف والمتقدمين", "Manage jobs and applicants", "pharmacy.job.view"),

		pharmacyPage("pharmacy.activity.view", g, "activities", "سجل العمليات والنشاط", "Activity log"),
	}
}

func pharmacyAccountPerms() []Permission {
	g := gPharmacyAccount
	return []Permission{
		// The Capsule assistant reads business data on the caller's behalf. It is
		// a separate grant from the screens it reads, so an owner can hand an
		// employee the dashboard without handing them a natural-language way to
		// summarise the whole company — and can withdraw it in one click.
		//
		// It never WIDENS access: every assistant tool also requires the same
		// permission the corresponding screen requires, so this grant alone
		// shows an employee nothing they could not already open.
		pharmacyAct("pharmacy.assistant.use", g,
			"استخدام الذكاء الاصطناعي (دكتور كبسولة)", "Use the Doctor Capsule AI assistant"),
		// Acting is a second, separate grant. With it, Capsule may PREPARE an
		// action this member could already take on the dashboard; the member
		// still confirms every one on a card that shows exactly what happens.
		// It never widens authority: each action also requires the permission
		// of the screen that performs it.
		pharmacyAct("pharmacy.assistant.act", g,
			"تنفيذ الإجراءات عبر دكتور كبسولة بعد التأكيد", "Let Doctor Capsule prepare actions for confirmation",
			"pharmacy.assistant.use"),

		pharmacyPage("pharmacy.ai_log.view", g, "ai_logs", "سجل استهلاك الذكاء الاصطناعي", "AI consumption log"),
		pharmacyPage("pharmacy.session.view", g, "sessions", "الأجهزة والجلسات النشطة", "Active sessions"),
		pharmacyAct("pharmacy.session.revoke", g, "إنهاء الجلسات", "Revoke sessions", "pharmacy.session.view"),
	}
}

// allGroups and allPermissions are what Default() assembles. They live here
// because this is the last of the three dashboard files; keeping them beside a
// catalogue file rather than in catalog.go means adding a dashboard is one new
// file plus two lines, not a hunt through the loader.
func allGroups() []Group {
	out := make([]Group, 0, 24)
	out = append(out, adminGroups()...)
	out = append(out, vendorGroups()...)
	out = append(out, pharmacyGroups()...)
	return out
}

func allPermissions() []Permission {
	out := make([]Permission, 0, 288)
	out = append(out, adminPermissions()...)
	out = append(out, vendorPermissions()...)
	out = append(out, pharmacyPermissions()...)
	return out
}
