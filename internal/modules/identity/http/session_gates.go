package http

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// Gates applied on every authenticated request, kept apart from RequireAuth so
// that middleware.go stays within its size limit.

// passwordChangeRedirect sends a session that must change its password to the
// sessions page, which carries the change form. The page and its form are the
// only ones such a session may reach.
func passwordChangeRedirect(sess *identity.Session, path string) (string, bool) {
	if sess == nil || !sess.MustChangePassword {
		return "", false
	}
	switch path {
	case "/customer/sessions", "/customer/password", "/vendor/sessions", "/vendor/password":
		return "", false
	}
	return passwordPagePath(sess.OrgType), true
}

func passwordPagePath(orgType string) string {
	if orgType == "vendor" {
		return "/vendor/sessions"
	}
	return "/customer/sessions"
}

func passwordPageKey(orgType string) string {
	if orgType == "vendor" {
		return "vendor.session.view"
	}
	return "pharmacy.session.view"
}

// memberOfActiveOrg reports whether the user may still act for the organization
// their session names. A member row that is removed or inactive does not count.
// An owner has no member row, so the owner check runs when the membership grant
// is empty. When the grant cannot be resolved the session's copy is kept, as the
// permission path does.
func memberOfActiveOrg(ctx context.Context, service *identity.Service, resolver *rbac.Resolver, userID, orgID int64) bool {
	grant, err := resolver.Resolve(ctx, userID, orgID)
	if err != nil || grant.OrgType != "" {
		return true
	}
	belongs, err := service.UserBelongsToOrg(ctx, userID, orgID)
	return err != nil || belongs
}

func hasKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}
