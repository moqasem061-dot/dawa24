package http

import (
	"context"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// Gates applied on every authenticated request, kept apart from RequireAuth so
// that middleware.go stays within its size limit.

// passwordChangeRedirect sends a session that must change its password to the
// account password page. That page asks for no permission, so a role without
// the sessions permission can still reach it. The page and the two form
// endpoints are the only ones such a session may reach.
func passwordChangeRedirect(sess *identity.Session, path string) (string, bool) {
	if sess == nil || !sess.MustChangePassword {
		return "", false
	}
	switch path {
	case "/account/password", "/customer/password", "/vendor/password":
		return "", false
	}
	return "/account/password", true
}

// holdForPasswordChange answers a request from a session that must change its
// password, and reports whether it did. The page's own timer polls (the
// notification badge) get an empty 204: a redirect would reload the page the
// user is on and wipe the password form they are typing in.
func holdForPasswordChange(w http.ResponseWriter, r *http.Request, sess *identity.Session) bool {
	target, held := passwordChangeRedirect(sess, r.URL.Path)
	if !held {
		return false
	}
	switch {
	case isBackgroundPoll(r):
		w.WriteHeader(http.StatusNoContent)
	case r.Header.Get("HX-Request") == "true":
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusUnauthorized)
	default:
		http.Redirect(w, r, target, http.StatusSeeOther)
	}
	return true
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
