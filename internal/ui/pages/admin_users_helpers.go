package pages

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
)

func userDisplayName(u *identity.User) string {
	if u.Name.Get("ar") != "" {
		return u.Name.Get("ar")
	}
	if u.Name.Get("en") != "" {
		return u.Name.Get("en")
	}
	return u.Email
}

func roleDisplayAr(role string) string {
	switch strings.ToLower(role) {
	case "super_admin":
		return "مدير نظام عام"
	case "admin":
		return "مدير منصة"
	case "pharmacy":
		return "صيدلية"
	case "supplier":
		return "مورد معتمد"
	case "vendor":
		return "بائع / مخزن"
	case "customer", "individual":
		return "عميل / مشتري"
	case "employer":
		return "مسؤول توظيف"
	default:
		return role
	}
}

func roleBadgeClass(role string) string {
	switch strings.ToLower(role) {
	case "super_admin", "admin":
		return "badge-danger"
	case "pharmacy":
		return "badge-sky"
	case "supplier", "vendor":
		return "badge-emerald"
	default:
		return "badge-slate"
	}
}

func userInitials(u *identity.User) string {
	if u == nil {
		return "U"
	}
	name := strings.TrimSpace(u.Name.Get("ar"))
	if name == "" {
		name = strings.TrimSpace(u.Name.Get("en"))
	}
	if name == "" {
		name = strings.TrimSpace(u.Email)
	}
	words := strings.Fields(name)
	if len(words) >= 2 {
		r1 := []rune(words[0])
		r2 := []rune(words[1])
		if len(r1) > 0 && len(r2) > 0 {
			return string(r1[0]) + string(r2[0])
		}
	}
	runes := []rune(name)
	if len(runes) > 0 {
		return string(runes[0])
	}
	return "U"
}

func adminUsersQueryValues(data AdminUsersPageData) url.Values {
	vals := url.Values{}
	if data.SearchQuery != "" {
		vals.Set("q", data.SearchQuery)
	}
	if data.RoleFilter != "" {
		vals.Set("role", data.RoleFilter)
	}
	if data.TypeFilter != "" {
		vals.Set("type", data.TypeFilter)
	}
	if data.StatusFilter != "" {
		vals.Set("status", data.StatusFilter)
	}
	if data.OrgFilter > 0 {
		vals.Set("org_id", fmt.Sprintf("%d", data.OrgFilter))
	}
	if data.MFAFilter != "" {
		vals.Set("mfa", data.MFAFilter)
	}
	if data.LoginFromFilter != "" {
		vals.Set("login_from", data.LoginFromFilter)
	}
	if data.LoginToFilter != "" {
		vals.Set("login_to", data.LoginToFilter)
	}
	return vals
}
