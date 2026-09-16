package rbac

// The account menu, declared beside the navigation and the permissions.
//
// It used to be eleven links hand-written into user_menu.templ, and not one of
// them asked whether the caller could open what it offered:
//
//   - "طلبات التوريد" pointed at /orders for suppliers. /orders is registered
//     inside the customer audience group, so every supplier who clicked it got
//     a 404 — the page does not exist for them at all; theirs is /vendor/orders.
//   - The wallet, the followed-suppliers list and the orders list were rendered
//     for every member of a company, including members holding none of
//     pharmacy.wallet.view, pharmacy.supplier.view or pharmacy.order.view. The
//     route gates answer those with a 404 as well.
//   - Platform staff were offered nothing at all beyond the dashboard.
//
// Only orgLink() checked anything, and it is the shape this file generalises:
// every entry names the permission its own route gate names, so the menu and
// the gate cannot disagree. AccountMenuAudit in the ui package walks this list
// and issues a real request for every href.

// AccountMenu returns the account-menu groups a caller may actually reach.
//
// approved is the organization's standing. A company still under review holds
// its dashboard permissions but must not be offered the dashboard, so the
// pending set is returned instead — the same rule the sidebar applies.
func AccountMenu(scope Scope, held Set, approved bool) []NavSection {
	var src []NavSection
	switch {
	case !approved:
		src = pendingMenu(scope)
	case scope == ScopeAdmin:
		src = adminMenu()
	case scope == ScopeVendor:
		src = vendorMenu()
	case scope == ScopePharmacy:
		src = pharmacyMenu()
	default:
		src = pendingMenu(scope)
	}
	return filterMenu(src, held)
}

// filterMenu keeps the items the holding reveals and the groups that still have
// one. It is VisibleNav's rule, applied to the menu, so a reader only has to
// learn it once.
func filterMenu(src []NavSection, held Set) []NavSection {
	out := make([]NavSection, 0, len(src))
	for _, sec := range src {
		items := make([]NavItem, 0, len(sec.Items))
		for _, it := range sec.Items {
			if it.Visible(held) {
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			sec.Items = items
			out = append(out, sec)
		}
	}
	return out
}

// accountSettingsItem is the same entry the sidebar carries, for the same
// reason: it is about the caller, not the company.
func accountSettingsItem() NavItem {
	return NavItem{
		Key: "account_settings", Href: "/settings", Icon: "settings",
		NameAr: "اعدادات الحساب", NameEn: "Account settings",
		AlwaysVisible: true,
	}
}

// reportIssueItem is available to every user on the platform to report problems
// or suggestions directly to technical support.
func reportIssueItem() NavItem {
	return NavItem{
		Key: "report_issue", Href: "/report-issue", Icon: "alert-circle",
		NameAr: "الإبلاغ عن مشكلة", NameEn: "Report an issue",
		AlwaysVisible: true,
	}
}

func pharmacyMenu() []NavSection {
	return []NavSection{
		{
			Key: "dashboard", NameAr: "لوحة التحكم", NameEn: "Dashboard",
			Items: []NavItem{
				{Key: "dashboard", Href: "/customer/dashboard", Icon: "grid",
					NameAr: "دخول اللوحة", NameEn: "Open dashboard",
					Perm: "pharmacy.dashboard.view"},
			},
		},
		{
			Key: "commerce", NameAr: "الحساب التجاري", NameEn: "Business account",
			Items: []NavItem{
				{Key: "orders", Href: "/orders", Icon: "package",
					NameAr: "طلباتي", NameEn: "My orders",
					Perm: "pharmacy.order.view"},
				{Key: "wallet", Href: "/customer/wallet", Icon: "wallet",
					NameAr: "المحفظة والرصيد", NameEn: "Wallet",
					Perm: "pharmacy.wallet.view"},
				{Key: "followed", Href: "/suppliers/followed", Icon: "heart-filled",
					NameAr: "الموردون المتابعون", NameEn: "Followed suppliers",
					Perm: "pharmacy.supplier.follow"},
			},
		},
		{
			Key: "account", NameAr: "الحساب", NameEn: "Account",
			Items: []NavItem{
				accountSettingsItem(),
				reportIssueItem(),
				{Key: "user_organization", Href: "/customer/user-organization", Icon: "building",
					NameAr: "المنشآت المرتبطة", NameEn: "Linked organizations",
					Perm: "pharmacy.user_org.view"},
				{Key: "documents", Href: "/customer/documents", Icon: "file",
					NameAr: "المستندات والتراخيص", NameEn: "Documents & licences",
					Perm: "pharmacy.document.view"},
			},
		},
	}
}

func vendorMenu() []NavSection {
	return []NavSection{
		{
			Key: "dashboard", NameAr: "لوحة التحكم", NameEn: "Dashboard",
			Items: []NavItem{
				{Key: "dashboard", Href: "/vendor/dashboard", Icon: "grid",
					NameAr: "دخول اللوحة", NameEn: "Open dashboard",
					Perm: "vendor.dashboard.view"},
			},
		},
		{
			Key: "delivery", NameAr: "إدارة الشحنات", NameEn: "Shipment dispatch",
			Items: []NavItem{
				{Key: "delivery", Href: "/vendor/delivery", Icon: "truck",
					NameAr: "إدارة الشحنات", NameEn: "Shipment dispatch",
					Perm: "vendor.delivery.view"},
			},
		},
		{
			Key: "commerce", NameAr: "الحساب التجاري", NameEn: "Business account",
			Items: []NavItem{
				// /vendor/orders, not /orders: the customer group owns /orders
				// and answers 404 to anyone who is not a pharmacy.
				{Key: "orders", Href: "/vendor/orders", Icon: "package",
					NameAr: "طلبات التوريد", NameEn: "Supply orders",
					Perm: "vendor.order.view"},
				{Key: "wallet", Href: "/vendor/wallet", Icon: "wallet",
					NameAr: "المحفظة والرصيد", NameEn: "Wallet",
					Perm: "vendor.wallet.view"},
				{Key: "organization", Href: "/vendor/organization", Icon: "building",
					NameAr: "بيانات المنشأة", NameEn: "Company profile",
					Perm: "vendor.organization.view"},
			},
		},
		{
			Key: "account", NameAr: "الحساب", NameEn: "Account",
			Items: []NavItem{
				accountSettingsItem(),
				reportIssueItem(),
				{Key: "user_organization", Href: "/vendor/user-organization", Icon: "users",
					NameAr: "المنشآت المرتبطة", NameEn: "Linked organizations",
					Perm: "vendor.user_org.view"},
				{Key: "documents", Href: "/vendor/documents", Icon: "file",
					NameAr: "المستندات والتراخيص", NameEn: "Documents & licences",
					Perm: "vendor.document.view"},
			},
		},
	}
}

func adminMenu() []NavSection {
	return []NavSection{
		{
			Key: "dashboard", NameAr: "لوحة التحكم", NameEn: "Dashboard",
			Items: []NavItem{
				{Key: "dashboard", Href: "/admin/dashboard", Icon: "grid",
					NameAr: "دخول اللوحة", NameEn: "Open dashboard",
					Perm: "platform.dashboard.view"},
			},
		},
		{
			Key: "operations", NameAr: "التشغيل", NameEn: "Operations",
			Items: []NavItem{
				{Key: "approvals", Href: "/admin/approvals", Icon: "shield",
					NameAr: "اعتماد المنشآت والوثائق", NameEn: "Approvals",
					Perm: "org.approval.view"},
				{Key: "organizations", Href: "/admin/organizations", Icon: "building",
					NameAr: "المنشآت والمؤسسات", NameEn: "Organizations",
					Perm: "org.organization.view"},
				{Key: "report_issues", Href: "/admin/report-issues", Icon: "alert-circle",
					NameAr: "إدارة البلاغات والشكاوى", NameEn: "Issue reports",
					Perm: "workflow.issue.view"},
				{Key: "notifications", Href: "/admin/notifications", Icon: "bell",
					NameAr: "مركز الإشعارات", NameEn: "Notifications",
					Perm: "notifications.center.view"},
			},
		},
		{
			Key: "account", NameAr: "الحساب", NameEn: "Account",
			Items: []NavItem{
				accountSettingsItem(),
				reportIssueItem(),
			},
		},
	}
}

// pendingMenu is what a caller sees while their organization is under review,
// rejected or suspended: the two things they can still act on, and their own
// account. Every dashboard destination is withheld, because RequireApproved
// would bounce them off it.
func pendingMenu(scope Scope) []NavSection {
	docs := "/customer/documents"
	if scope == ScopeVendor {
		docs = "/vendor/documents"
	}
	return []NavSection{
		{
			Key: "pending", NameAr: "قيد المراجعة", NameEn: "Under review",
			Items: []NavItem{
				{Key: "documents", Href: docs, Icon: "file",
					NameAr: "المستندات والتراخيص", NameEn: "Documents & licences",
					AlwaysVisible: true},
				{Key: "notifications", Href: "/notifications", Icon: "bell",
					NameAr: "مركز الإشعارات", NameEn: "Notifications centre",
					AlwaysVisible: true},
			},
		},
		{
			Key: "account", NameAr: "الحساب", NameEn: "Account",
			Items: []NavItem{
				accountSettingsItem(),
				reportIssueItem(),
			},
		},
	}
}
