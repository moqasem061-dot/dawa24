package authctx

import (
	"log/slog"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The buying audience.
//
// /customer/* and /vendor/* are two dashboards and their gates keep them apart.
// The catalogue, the supplier directory, the offers board, the purchase
// request, the cart and the buyer's own orders are neither: they are the one
// marketplace both companies buy through, and a supplier restocking from
// another distributor uses exactly the pages a pharmacy uses.
//
// So they get their own audience. RequireBuyer says who may be on the surface
// at all; RequireBuyingPagePermission says which of its pages, resolved against
// the caller's own dashboard so a supplier is judged by their vendor. grant and
// a pharmacist by their pharmacy. one.

// IsBuyer reports whether the actor may reach the shared buying surface: a
// member of a company, on either dashboard.
//
// Platform staff are excluded. They hold no tenant grants and have no company
// to buy for; a cart belonging to "the platform" is not a thing that exists.
func (a Actor) IsBuyer() bool {
	if a.IsStaff {
		return false
	}
	if a.OrganizationID <= 0 && a.OrgID <= 0 {
		return false
	}
	switch a.DashboardScope() {
	case rbac.ScopeVendor, rbac.ScopePharmacy:
		return true
	}
	return false
}

// RequireBuyer gates the shared buying surface. A caller who is not a member of
// a buying company is sent to whatever dashboard they do belong to.
func RequireBuyer(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			if !actor.IsBuyer() {
				log.WarnContext(r.Context(), "buying audience denied",
					"path", r.URL.Path, "user_id", actor.UserID,
					"org_type", actor.OrgType, "is_staff", actor.IsStaff)
				redirectUnauthorized(w, r, actor)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCapability gates one page that both dashboards share. Holding any one
// of the named capabilities is enough, for a page two roles reach for different
// reasons.
//
// The capability names the page once; the key checked is the one the caller's
// own dashboard grants it under. That is what keeps the two sidebars and this
// gate from drifting: there is one declaration, in rbac/capability.go, and both
// read it.
//
// It is the third gate beside RequirePagePermission (staff only) and
// RequireTenantPagePermission (one dashboard, one key). Those two could not
// express a shared page: naming both keys in a tenant gate would have let a
// pharmacy key satisfy a supplier's request, and naming one would have locked
// the other dashboard out.
func RequireCapability(caps ...rbac.Capability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			if !actor.IsBuyer() {
				redirectUnauthorized(w, r, actor)
				return
			}
			required := rbac.RequiredKeys(actor.DashboardScope(), caps...)
			// No key for this dashboard means the capability is not offered
			// here at all. Refuse rather than fall through to CanAny, which
			// treats an empty requirement as ungated.
			if len(required) > 0 && actor.CanAny(required...) {
				next.ServeHTTP(w, r)
				return
			}
			denied(r, "buying", actor, required)
			redirectUnauthorized(w, r, actor)
		})
	}
}
