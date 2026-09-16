package rbac

import "sort"

// SystemRole is a role the platform ships and an operator may not delete.
//
// Two families live here. Platform roles sit in identity.roles and name a
// user's standing on the platform (identity.users.role). Organization roles
// are seeded into every company's own org.roles so that a vendor or pharmacy
// owner starts with something usable and can copy it.
type SystemRole struct {
	Key    string
	NameAr string
	NameEn string
	DescAr string
	// Scope is the dashboard this role governs. Platform roles are ScopeAdmin;
	// an organization role is seeded per company into the scope matching its
	// organization type.
	Scope Scope
	// IsStaff marks a platform role whose holders reach /admin/*. It replaces
	// the hardcoded four-name list that Session.IsStaff used to carry, which
	// meant a new moderator role could not be staff without a code change.
	IsStaff bool
	// Owner marks the role that holds everything in its scope. Its permission
	// list is computed from the catalogue rather than enumerated, so a new
	// page is owned the moment it is declared.
	Owner bool
	// Permissions is the explicit grant list for a non-owner role.
	Permissions []string
	// TenantScopes restricts which company dashboards an organization role is
	// seeded into. Empty means every tenant dashboard, which is the case for
	// the six roles that describe a job both a supplier and a pharmacy have.
	//
	// It exists for the roles that describe a job only one of them has. A
	// مندوب توصيل carries a supplier's parcels to a pharmacy; a pharmacy has
	// no such employee, and seeding the role there would put a permanently
	// empty role in every pharmacy's role editor.
	TenantScopes []Scope
}

// SeededIn reports whether this organization role belongs in a company on the
// given dashboard.
func (r SystemRole) SeededIn(scope Scope) bool {
	if len(r.TenantScopes) == 0 {
		return true
	}
	for _, s := range r.TenantScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// PlatformRoles are the roles a user account may hold on the platform itself.
//
// super_admin is the only one with Owner set. admin, support and developer are
// ordinary staff roles now: their permissions come from this list and a super
// admin may edit them, which is the whole point of the exercise — previously
// "admin" was a bypass in four different if-statements.
func PlatformRoles() []SystemRole {
	return []SystemRole{
		{
			Key: "super_admin", NameAr: "مدير النظام الأعلى", NameEn: "Super Admin",
			DescAr: "صلاحية كاملة على كل صفحات وأقسام لوحة الإدارة.",
			Scope:  ScopeAdmin, IsStaff: true, Owner: true,
		},
		{
			Key: "admin", NameAr: "مدير", NameEn: "Administrator",
			DescAr: "إدارة تشغيلية كاملة عدا أدوات المطوّر وإدارة الأدوار.",
			Scope:  ScopeAdmin, IsStaff: true,
			Permissions: adminRoleGrants(),
		},
		{
			Key: "support", NameAr: "الدعم الفني", NameEn: "Support",
			DescAr: "قراءة بيانات المنشآت والطلبات والرد على الرسائل والبلاغات.",
			Scope:  ScopeAdmin, IsStaff: true,
			Permissions: []string{
				"platform.dashboard.view", "notifications.center.view",
				"org.organization.view", "org.branch.view", "org.review.view",
				"identity.user.view", "identity.activity.view", "platform.chat.view",
				"catalog.product.view", "catalog.category.view", "catalog.brand.view",
				"commerce.order.view", "commerce.quote.view",
				"billing.invoice.view", "billing.invoice.read",
				"promo.offer.view", "promo.ad.view",
				"platform.message.view", "platform.message.update",
				"workflow.request.view", "workflow.request.update",
				"workflow.issue.view", "workflow.issue.update",
				"hr.job.view", "hr.document.view", "org.approval.view",
			},
		},
		{
			Key: "developer", NameAr: "مطوّر", NameEn: "Developer",
			DescAr: "أدوات التشخيص والسجلات وواجهات المنصة البرمجية.",
			Scope:  ScopeAdmin, IsStaff: true,
			Permissions: []string{
				"platform.dashboard.view",
				"platform.developer.sql", "platform.admin",
				"platform.error_log.view", "platform.error_log.update", "platform.error_log.delete",
				"platform.activity_log.view", "platform.activity_log.delete",
				"platform.ai.view", "platform.analytics.view",
				"platform.setting.view", "platform.translation.view",
				"catalog.product.view", "org.organization.view", "identity.user.view",
			},
		},
		{
			// A main moderator: uploads temporary warehouses of their own AND
			// oversees the moderators assigned under them.
			//
			// "Main" is not a separate role. Any moderator with no parent is a
			// main moderator, and one with a parent is a sub-moderator; the
			// distinction is a column on the user, not a second role to keep in
			// step. Both hold the same permissions, and what each of them
			// actually sees is decided by the hierarchy query — which is the
			// only place it can be decided correctly, because a moderator with
			// no subordinates today may have four tomorrow.
			Key: "moderator", NameAr: "مشرف", NameEn: "Moderator",
			DescAr: "رفع وإدارة المستودعات المؤقتة، ومتابعة المشرفين المعيّنين تحت إدارته.",
			Scope:  ScopeAdmin, IsStaff: true,
			Permissions: []string{
				"platform.dashboard.view",
				"notifications.center.view",
				"inventory.my_temp_warehouse.view",
				"inventory.my_temp_warehouse.manage",
				"inventory.team_temp_warehouse.view",
				"inventory.team_temp_warehouse.manage",
				"catalog.product.view",
			},
		},
		{
			// The ordinary account role. It grants nothing on the platform:
			// a member's capability inside their company comes from their
			// membership, never from this column (Rebuild V2 rule 1).
			Key: "user", NameAr: "مستخدم", NameEn: "User",
			DescAr: "حساب عادي؛ صلاحياته داخل لوحة المنشأة تأتي من عضويته وليس من دوره على المنصة.",
			Scope:  ScopePharmacy,
		},
		{
			Key: "customer", NameAr: "عميل", NameEn: "Customer",
			DescAr: "مرادف تاريخي لدور المستخدم العادي، محفوظ لتوافق البيانات.",
			Scope:  ScopePharmacy,
		},
		{
			Key: "job_seeker", NameAr: "باحث عن عمل", NameEn: "Job Seeker",
			DescAr: "حساب باحث عن عمل بدون صلاحيات إدارية.",
			Scope:  ScopePharmacy,
		},
	}
}

// adminRoleGrants is everything on the admin dashboard except the developer
// tools and role administration. Computing it by subtraction rather than
// enumeration means a new page is included automatically, which is the
// behaviour an operator expects from a role called "Administrator".
func adminRoleGrants() []string {
	withheld := map[string]struct{}{
		"platform.developer.sql":           {},
		"platform.admin":                   {},
		"platform.page_control.create":     {},
		"platform.page_control.update":     {},
		"platform.page_control.delete":     {},
		"platform.error_log.delete":        {},
		"platform.activity_log.delete":     {},
		"identity.admin_role.update":       {},
		"identity.admin_role.delete":       {},
		"identity.admin_role.assign":       {},
		"platform.trash.purge":             {},
		"org.organization.delete":          {},
		"identity.user.delete":             {},
		"billing.wallet.manage":            {},
		"billing.subscription_plan.update": {},
		// Acting through the assistant is granted deliberately, by a super
		// admin, never inherited by a whole role.
		"platform.assistant.act": {},
	}
	all := Default().KeysFor(ScopeAdmin)
	out := make([]string, 0, len(all))
	for _, k := range all {
		if _, no := withheld[k]; no {
			continue
		}
		out = append(out, k)
	}
	return out
}

// OrganizationRoles are the starter roles seeded into every company. The same
// six keys exist for a vendor and for a pharmacy; the permissions differ
// because the dashboards differ, so each is resolved against the company's own
// scope by GrantsFor.
func OrganizationRoles() []SystemRole {
	return []SystemRole{
		{
			Key: "org_owner", NameAr: "مالك المنشأة", NameEn: "Owner",
			DescAr: "صلاحية كاملة على كل صفحات وأقسام لوحة المنشأة.",
			Owner:  true,
		},
		{Key: "org_manager", NameAr: "مدير", NameEn: "Manager",
			DescAr: "إدارة العمليات اليومية عدا الأدوار والاشتراك والمحفظة."},
		{Key: "org_accountant", NameAr: "محاسب", NameEn: "Accountant",
			DescAr: "الفواتير والمدفوعات والمحفظة والأرباح."},
		{Key: "org_warehouse", NameAr: "أمين مخزن", NameEn: "Warehouse Keeper",
			DescAr: "المخزون والمخازن والاستيراد."},
		{Key: "org_sales_rep", NameAr: "مندوب مبيعات", NameEn: "Sales Representative",
			DescAr: "الطلبات والعروض ومتابعة العملاء."},
		{Key: "org_pharmacist", NameAr: "صيدلي مسؤول", NameEn: "Responsible Pharmacist",
			DescAr: "الشراء ومتابعة الطلبات والأصناف."},
		{Key: "org_employee", NameAr: "موظف", NameEn: "Employee",
			DescAr: "اطلاع فقط على لوحة التحكم والطلبات."},
		// The delivery representative. Supplier-only: it is the person who
		// carries a parcel from the supplier's warehouse to the pharmacy's
		// counter, and a pharmacy employs nobody who does that.
		//
		// Their dashboard is one page — إدارة الشحنات — showing the parcels
		// assigned to them, oldest assignment first, with the actions needed
		// to close each one. They hold nothing else: not the order list the
		// parcels came from, not the catalogue, not the wallet.
		{Key: "org_courier", NameAr: "مندوب توصيل", NameEn: "Delivery Representative",
			DescAr:       "بوابة إدارة الشحنات: الطرود المسندة إليه فقط، وتحديث حالتها وتأكيد تسليمها بالكود.",
			TenantScopes: []Scope{ScopeVendor}},
	}
}

// GrantsFor resolves a system role's permission keys within a scope, expanded
// by implication and restricted to that scope. An owner role gets everything
// the scope declares.
func GrantsFor(role SystemRole, scope Scope) []string {
	c := Default()
	if role.Owner {
		return c.KeysFor(scope)
	}
	if role.Permissions != nil {
		return c.Restrict(role.Permissions, scope)
	}
	if byKey, ok := orgRoleGrants[scope]; ok {
		if keys, ok := byKey[role.Key]; ok {
			return c.Restrict(keys, scope)
		}
	}
	return nil
}

// PlatformRole returns a platform role definition by key.
func PlatformRole(key string) (SystemRole, bool) {
	for _, r := range PlatformRoles() {
		if r.Key == key {
			return r, true
		}
	}
	return SystemRole{}, false
}

// OrganizationRole returns an organization starter role definition by key.
func OrganizationRole(key string) (SystemRole, bool) {
	for _, r := range OrganizationRoles() {
		if r.Key == key {
			return r, true
		}
	}
	return SystemRole{}, false
}

// OrganizationRolesFor lists the starter roles a company on this dashboard is
// seeded with. It is the one place the TenantScopes filter is applied, so the
// seeder, the repair pass and the tests cannot disagree about which roles a
// supplier has and a pharmacy does not.
func OrganizationRolesFor(scope Scope) []SystemRole {
	all := OrganizationRoles()
	out := make([]SystemRole, 0, len(all))
	for _, r := range all {
		if r.SeededIn(scope) {
			out = append(out, r)
		}
	}
	return out
}

// SystemRoleKeys lists the organization starter role keys, sorted, for the
// seeder and for tests.
func SystemRoleKeys() []string {
	out := make([]string, 0, 8)
	for _, r := range OrganizationRoles() {
		out = append(out, r.Key)
	}
	sort.Strings(out)
	return out
}
