package authctx

import (
	"log/slog"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RequirePermission ensures the authenticated actor possesses the required
// permission. Pass several and holding any one of them is enough.
//
// There is no role-name bypass. It used to read
// `actor.Role == "super_admin" || actor.Role == "developer"`, which meant two
// role names were wired into every gate: a new staff role could not be given
// full access without editing this function, and the developer role held every
// API in the platform whatever its grants said. Both now hold their access as
// permissions — super_admin holds the whole catalogue, which reaches the same
// outcome through the system instead of around it.
func RequirePermission(permissionKeys ...string) func(http.Handler) http.Handler {
	log := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			if !actor.CanAny(permissionKeys...) {
				denied(r, "api", actor, permissionKeys)
				httpx.Error(w, r, log, apperr.Forbidden(
					"auth.forbidden",
					"You do not have permission to perform this action.",
				))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole ensures the authenticated actor holds one of the allowed roles.
//
// Prefer RequirePermission. A role name says who someone is; a permission says
// what they may do, and only the second survives an operator inventing a role
// this code has never heard of. This remains for the few checks that are
// genuinely about identity rather than capability.
func RequireRole(allowedRoles []string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			if actor.Role == "super_admin" || actor.Role == "developer" {
				next.ServeHTTP(w, r)
				return
			}

			hasRole := false
			for _, r := range allowedRoles {
				if actor.Role == r {
					hasRole = true
					break
				}
			}

			if !hasRole {
				httpx.Error(w, r, log, apperr.Forbidden(
					"auth.forbidden",
					"Your role is not authorized for this resource.",
				))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
