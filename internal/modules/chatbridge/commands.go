package chatbridge

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Markup is a channel's text formatting. Bold and Code receive text that has
// already been through Escape.
type Markup interface {
	Escape(s string) string
	Bold(s string) string
	Code(s string) string
}

// WhoAmI describes the chat's live account, منشأة, branch, role and whether
// the assistant is available to it.
func (c *Core) WhoAmI(ctx context.Context, chat *Chat) (string, error) {
	sys := database.AsSystem(ctx)
	m := c.Markup
	actor, refusal, err := c.Resolve(ctx, chat)
	if err != nil || refusal != "" {
		return refusal, err
	}
	memberships, err := c.Store.Memberships(sys, chat.UserID)
	if err != nil {
		return "", err
	}
	grant, err := c.Grants.Resolve(sys, actor.UserID, actor.OrgID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("👤 " + m.Bold("الحساب:") + " " + m.Escape(orDash(actor.Name)) + "\n")
	current := membershipByOrg(memberships, actor.OrgID)
	switch {
	case current != nil:
		fmt.Fprintf(&b, "🏢 %s %s — %s\n", m.Bold("المنشأة:"), m.Escape(current.Name), orgTypeLabel(current.Type))
		b.WriteString("📋 " + m.Bold("حالة المنشأة:") + " " + orgStatusLabel(m, current.Status) + "\n")
		branch := "كل فروع المنشأة"
		if current.BranchName != "" {
			branch = current.BranchName + " (صلاحياتك مقصورة على هذا الفرع)"
		}
		b.WriteString("🏬 " + m.Bold("الفرع:") + " " + m.Escape(branch) + "\n")
	case actor.IsStaff:
		b.WriteString("🏢 " + m.Bold("النطاق:") + " إدارة المنصة\n")
	}
	b.WriteString("🔐 " + m.Bold("الدور:") + " " + m.Escape(roleLabel(grant)) + "\n")

	assistantState := "متاح ✅"
	if ApprovalRefusal(actor) != "" {
		assistantState = "غير متاح — المنشأة غير معتمدة"
	} else if !c.Assistant.Allowed(actor) {
		assistantState = "غير مفعّل لدورك 🔒"
	}
	b.WriteString("🤖 " + m.Bold("المساعد كبسولة:") + " " + assistantState + "\n")
	if len(memberships) > 1 {
		b.WriteString("\nلديك " + strconv.Itoa(len(memberships)) + " منشآت. للتبديل: /org")
	}
	return b.String(), nil
}

// Org lists the user's organisations, or switches to one of them.
//
// The list is re-read at each call and the choice is an index into it, so the
// only organisations that can be selected are ones the user is an active
// member of right now. Selection still authorises nothing: the next message
// re-resolves membership and permissions for the chosen منشأة.
func (c *Core) Org(ctx context.Context, chat *Chat, arg string) (string, error) {
	sys := database.AsSystem(ctx)
	m := c.Markup
	memberships, err := c.Store.Memberships(sys, chat.UserID)
	if err != nil {
		return "", err
	}
	platform, err := c.Grants.Resolve(sys, chat.UserID, 0)
	if err != nil {
		return "", err
	}
	if !platform.Active {
		return "🚫 حسابك في Dawa24 غير نشط حالياً.", nil
	}
	canPlatform := platform.IsStaff || platform.IsPlatformOwner

	if arg == "" {
		if len(memberships) == 0 && !canPlatform {
			return "لا توجد منشآت نشطة مرتبطة بحسابك.", nil
		}
		var b strings.Builder
		b.WriteString("🏢 " + m.Bold("اختر المنشأة التي يعمل ضمنها المساعد:") + "\n\n")
		if canPlatform {
			b.WriteString("0. إدارة المنصة" + currentMark(chat.OrgID() == 0) + "\n")
		}
		for i, ms := range memberships {
			fmt.Fprintf(&b, "%d. %s — %s%s\n", i+1, m.Escape(ms.Name), orgTypeLabel(ms.Type), currentMark(ms.OrgID == chat.OrgID()))
		}
		b.WriteString("\nللتبديل أرسل مثلاً: " + m.Code("/org 1"))
		return b.String(), nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(arg))
	var target *int64
	label := ""
	switch {
	case err == nil && n == 0 && canPlatform:
		label = "إدارة المنصة"
	case err == nil && n >= 1 && n <= len(memberships):
		id := memberships[n-1].OrgID
		target, label = &id, memberships[n-1].Name
	default:
		return "رقم غير صحيح. أرسل /org لعرض القائمة.", nil
	}
	if err := c.Store.SetActiveOrganization(sys, chat.LinkID, target); err != nil {
		return "", err
	}
	chat.ActiveOrgID, chat.ConversationID = target, nil
	return "✅ أصبحت المنشأة النشطة: " + m.Bold(m.Escape(label)) + "\nبدأت محادثة جديدة ضمن هذه المنشأة.", nil
}

// Notify shows or changes which notification categories reach the channel.
func (c *Core) Notify(ctx context.Context, chat *Chat, arg string) (string, error) {
	sys := database.AsSystem(ctx)
	m := c.Markup
	fields := strings.Fields(strings.ToLower(arg))
	if len(fields) == 2 && (fields[1] == "on" || fields[1] == "off") {
		next, ok := toggleMuted(chat.MutedCategories, fields[0], fields[1] == "off")
		if !ok {
			return "فئة غير معروفة. أرسل /notify لعرض الفئات.", nil
		}
		if err := c.Store.SetMutedCategories(sys, chat.LinkID, next); err != nil {
			return "", err
		}
		chat.MutedCategories = next
	} else if arg != "" {
		return "الصيغة: " + m.Code("/notify orders off") + " أو " + m.Code("/notify all on"), nil
	}

	offers, err := c.Store.OffersTopicEnabled(sys, chat.UserID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("🔔 " + m.Bold("إشعارات "+c.ChannelName) + "\n")
	b.WriteString("تصلك فقط الإشعارات التي تسمح بها صلاحياتك في Dawa24.\n\n")
	for _, cat := range Categories {
		state, toggle := "✅", "off"
		if Muted(chat.MutedCategories, cat) {
			state, toggle = "🔕", "on"
		}
		fmt.Fprintf(&b, "%s %s — %s\n", state, cat.Label(), m.Code("/notify "+string(cat)+" "+toggle))
	}
	if !offers {
		b.WriteString("\nℹ️ العروض الترويجية متوقفة من تفضيلات حسابك في Dawa24، ولن تصل هنا أيضاً.")
	}
	return b.String(), nil
}

// toggleMuted applies "<category|all> on|off" to a muted set, in the order of
// Categories. ok is false for an unknown category.
func toggleMuted(current []string, key string, off bool) ([]string, bool) {
	muted := map[string]bool{}
	for _, c := range current {
		muted[c] = true
	}
	if key == "all" {
		for _, c := range Categories {
			muted[string(c)] = off
		}
	} else if c, ok := ParseCategory(key); ok {
		muted[string(c)] = off
	} else {
		return nil, false
	}
	var next []string
	for _, c := range Categories {
		if muted[string(c)] {
			next = append(next, string(c))
		}
	}
	return next, true
}

func membershipByOrg(ms []Membership, orgID int64) *Membership {
	for i := range ms {
		if ms[i].OrgID == orgID {
			return &ms[i]
		}
	}
	return nil
}

func currentMark(current bool) string {
	if current {
		return " ✅ (الحالية)"
	}
	return ""
}

func roleLabel(g rbac.Grant) string {
	switch {
	case g.IsPlatformOwner:
		return "مالك المنصة"
	case g.IsStaff:
		return "فريق إدارة المنصة"
	case g.IsOrgOwner:
		return "مالك المنشأة"
	case g.MemberRoleName != "":
		return g.MemberRoleName
	}
	return "عضو"
}

func orgTypeLabel(t string) string {
	switch rbac.NormalizeOrgType(t) {
	case "vendor":
		return "مورّد"
	case "customer":
		return "صيدلية"
	}
	return "منشأة"
}

func orgStatusLabel(m Markup, s string) string {
	switch s {
	case "approved", "active", "verified":
		return "معتمدة ✅"
	case "pending", "under_review":
		return "قيد المراجعة ⏳"
	case "suspended":
		return "موقوفة 🚫"
	case "rejected":
		return "مرفوضة 🚫"
	}
	return m.Escape(s)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
