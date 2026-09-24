package datasets

import (
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// team never selects salary columns: the team screen's view permission does
// not show pay, and nothing here should either.
func team(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "team", Label: "فريق العمل", Scope: scope,
		Description: "Members of this organisation: role, branch, status.",
		Permissions: perms,
		From: `org.members m JOIN identity.users u ON u.id = m.user_id
			LEFT JOIN org.branches b ON b.id = m.branch_id
			LEFT JOIN org.roles r ON r.id = m.org_role_id`,
		Tenant: `m.organization_id = @org`,
		Fields: []Field{
			search("name", "الاسم", personName("u")),
			search("email", "البريد", `u.email`),
			hidden(text("phone", "الهاتف", `u.phone`)),
			search("role", "الدور", `COALESCE(NULLIF(`+localized("r.name")+`,''), m.role_key)`),
			search("job_title", "المسمى الوظيفي", `m.job_title`),
			text("branch", "الفرع", localized("b.name")),
			text("status", "الحالة", `m.status`),
			flag("active", "نشط", `m.is_active`),
			instant("joined_at", "تاريخ الانضمام", `COALESCE(m.joined_at, m.created_at)`),
		},
		DefaultSort: "joined_at",
	}
}

func subscriptions(scope rbac.Scope, perms []string) Dataset {
	return Dataset{
		Name: "subscriptions", Label: "الاشتراكات", Scope: scope,
		Description: "Subscription history of this organisation. The current plan is the latest starts_at.",
		Permissions: perms,
		From:        `billing.subscriptions s LEFT JOIN billing.plans p ON p.id = s.plan_id`,
		Tenant:      `s.organization_id = @org`,
		Fields: []Field{
			search("plan", "الباقة", localized("p.name")),
			text("status", "الحالة", `s.status`),
			text("billing_cycle", "دورة الفوترة", `s.billing_cycle`),
			flag("auto_renew", "تجديد تلقائي", `s.auto_renew`),
			instant("starts_at", "تبدأ", `s.starts_at`),
			instant("expires_at", "تنتهي", `s.expires_at`),
		},
		DefaultSort: "starts_at",
	}
}
