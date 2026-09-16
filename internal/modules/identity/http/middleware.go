package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type ctxKey int

const (
	ctxKeySession ctxKey = iota
)

// WithSession stores the active session in context.
func WithSession(ctx context.Context, sess *identity.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, sess)
}

// SessionFrom retrieves the active session from context.
func SessionFrom(ctx context.Context) (*identity.Session, bool) {
	sess, ok := ctx.Value(ctxKeySession).(*identity.Session)
	return sess, ok && sess != nil
}

// RequireAuth authenticates incoming requests via Authorization Bearer token or session cookie.
func RequireAuth(service *identity.Service, resolver *rbac.Resolver, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authctx.From(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}
			token := extractToken(r, cookieName)
			if token == "" {
				if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
					q := url.Values{}
					q.Set("redirect", r.URL.RequestURI())
					http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
					return
				}
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			// A poll the page issues by itself is not the user doing something,
			// so it must not keep the session alive past the idle timeout.
			sess, err := validateForRequest(service, r, token)
			if err != nil {
				// Expired/invalid session: clear cookie
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					Expires:  time.Unix(0, 0),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})

				if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
					q := url.Values{}
					q.Set("redirect", r.URL.RequestURI())
					var appErr *apperr.Error
					if errors.Is(err, identity.ErrSessionIdleTimeout) || (errors.As(err, &appErr) && appErr.Code == "session.idle_timeout") {
						q.Set("reason", "idle_timeout")
					} else if errors.Is(err, identity.ErrSessionEvictedDuplicateLogin) || (errors.As(err, &appErr) && appErr.Code == "session.evicted_duplicate_login") {
						q.Set("error", "duplicate_login")
					} else if errors.Is(err, identity.ErrSessionEvictedConcurrentLimit) || (errors.As(err, &appErr) && appErr.Code == "session.evicted_concurrent_limit") {
						q.Set("error", "concurrent_limit")
					}
					http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
					return
				}
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			// The session's active organization is the trusted default. A zero
			// org means the user has no organization membership (or is staff),
			// and no query then runs under a guessed tenant — the previous
			// hardcoded fallback to organization 1 handed a pharmacy account
			// another tenant's data via RLS scoping.
			activeOrgID := sess.ActiveOrgID
			if activeOrgID == 0 && !sess.IsStaff() && service != nil {
				if o, t, st, err := service.DefaultOrgInfoForUser(r.Context(), sess.UserID); err == nil && o > 0 {
					activeOrgID = o
					sess.ActiveOrgID = o
					sess.OrgType = t
					sess.OrgStatus = st
				}
			}

			ctx := WithSession(r.Context(), sess)
			if activeOrgID > 0 {
				ctx = database.WithTenant(ctx, activeOrgID)
			}
			// Publish the caller through the platform-level actor context so
			// other modules can identify them without importing this one, and
			// without trusting anything in the request.
			//
			// OrgType and OrgStatus come from the session, which login built
			// from org.members joined to org.organizations — never from the
			// request (Rebuild V2 §1.2).
			ctx = authctx.WithActor(ctx, actorFor(ctx, resolver, sess, activeOrgID, log))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// actorFor builds the authenticated caller.
//
// Permissions come from the resolver, which reads them now, rather than from
// the copy the session was stamped with at login. That copy was the whole
// problem: sessions last 720 hours, so revoking a role had no effect on anyone
// already signed in until they happened to log out. A permission system that
// cannot revoke within the session lifetime is not enforcing anything.
//
// If the resolver is unavailable — it is not wired in tests, and the database
// can be down — the caller falls back to the session's copy. That is the
// conservative choice in the wrong direction only for revocation; the
// alternative, denying everything, turns a database blip into a total outage
// for signed-in users.
func actorFor(ctx context.Context, resolver *rbac.Resolver, sess *identity.Session, orgID int64, log *slog.Logger) authctx.Actor {
	actor := authctx.Actor{
		UserID:         sess.UserID,
		OrganizationID: orgID,
		OrgID:          orgID,
		OrgType:        sess.OrgType,
		OrgStatus:      sess.OrgStatus,
		Role:           sess.Role,
		IsStaff:        sess.IsStaff(),
		Email:          sess.Email,
	}
	actor.Grants(sess.Permissions)

	if resolver == nil {
		if scope, ok := rbac.TenantScopeFor(sess.OrgType); ok {
			actor.Scope = scope
		} else if actor.IsStaff {
			actor.Scope = rbac.ScopeAdmin
		}
		return actor
	}

	grant, err := resolver.Resolve(ctx, sess.UserID, orgID)
	if err != nil {
		log.ErrorContext(ctx, "could not resolve permissions; falling back to the session copy",
			"error", err, "user_id", sess.UserID, "organization_id", orgID)
		return actor
	}
	authctx.ApplyGrant(&actor, grant)
	return actor
}

// OptionalAuth authenticates incoming requests if a valid session exists, but allows anonymous requests.
func OptionalAuth(service *identity.Service, resolver *rbac.Resolver, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r, cookieName)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// A poll the page issues by itself is not the user doing something,
			// so it must not keep the session alive past the idle timeout.
			sess, err := validateForRequest(service, r, token)
			if err != nil {
				// Expired or invalid session: clear cookie so browser stops sending dead token
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					Expires:  time.Unix(0, 0),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				next.ServeHTTP(w, r)
				return
			}

			// Same rule as RequireAuth: the session's active organization is
			// the only tenant default; there is no guessed fallback.
			activeOrgID := sess.ActiveOrgID
			if activeOrgID == 0 && !sess.IsStaff() && service != nil {
				if o, t, st, err := service.DefaultOrgInfoForUser(r.Context(), sess.UserID); err == nil && o > 0 {
					activeOrgID = o
					sess.ActiveOrgID = o
					sess.OrgType = t
					sess.OrgStatus = st
				}
			}

			ctx := WithSession(r.Context(), sess)
			if activeOrgID > 0 {
				ctx = database.WithTenant(ctx, activeOrgID)
			}
			ctx = authctx.WithActor(ctx, actorFor(ctx, resolver, sess, activeOrgID, log))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveTenant resolves the active tenant organization and binds it to database context.
func ResolveTenant(service *identity.Service, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, hasSession := SessionFrom(ctx)

			// The session's active organization is the trusted default.
			var orgID int64
			if hasSession {
				orgID = sess.ActiveOrgID
			}

			// X-Dawa-Org-ID lets a user who belongs to several organizations
			// switch between them. It is a request-supplied value, so it is
			// honoured only after confirming membership. Trusting it directly
			// would hand any caller another tenant's data, because row-level
			// security scopes to whatever organization it is told.
			if headerOrg := strings.TrimSpace(r.Header.Get("X-Dawa-Org-ID")); headerOrg != "" {
				requested, err := strconv.ParseInt(headerOrg, 10, 64)
				switch {
				case err != nil || requested <= 0:
					httpx.Error(w, r, log, apperr.Validation("org.invalid",
						"X-Dawa-Org-ID must be a positive integer.", nil))
					return
				case !hasSession:
					httpx.Error(w, r, log, apperr.Unauthorized())
					return
				case requested != orgID:
					newType, newStatus, newPerms, err := service.GetOrgInfoForUser(ctx, sess.UserID, requested)
					if err != nil {
						httpx.Error(w, r, log, err)
						return
					}
					orgID = requested
					if actor, ok := authctx.From(ctx); ok {
						actor.OrganizationID = orgID
						actor.OrgType = newType
						actor.OrgStatus = newStatus
						actor.Permissions = newPerms
						ctx = authctx.WithActor(ctx, actor)
					}
				}
			}

			if orgID > 0 {
				ctx = database.WithTenant(ctx, orgID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))

		})
	}
}

func extractToken(r *http.Request, cookieName string) string {
	// 1. Authorization: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Cookie
	if cookie, err := r.Cookie(cookieName); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}

	return ""
}

// backgroundPollPaths are the endpoints a logged-in page requests on a timer,
// with nobody necessarily at the keyboard. Reaching one of them proves the tab
// is open, not that the user is present, so it reads the session without
// refreshing its idle clock.
//
// Keep this in step with the templates: any `hx-trigger="... every Ns"` or
// setInterval-driven fetch belongs here, or it silently disables the 30-minute
// idle logout for every user who leaves that page open — which is exactly what
// the notification badge poll used to do.
var backgroundPollPaths = map[string]bool{
	"/notifications/unread-badge": true,
}

// validateForRequest reads the session, counting the request as user activity
// only when the user actually made it.
func validateForRequest(service *identity.Service, r *http.Request, token string) (*identity.Session, error) {
	if isBackgroundPoll(r) {
		return service.ValidateSessionWithoutTouch(r.Context(), token)
	}
	return service.ValidateSession(r.Context(), token)
}

// isBackgroundPoll reports whether this request was issued by the page on a
// timer rather than by the user.
func isBackgroundPoll(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	path := r.URL.Path
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	if backgroundPollPaths[path] {
		return true
	}
	// An explicit opt-in for callers that are not a fixed path (SSE streams,
	// progress pollers on a per-session URL).
	return r.Header.Get("X-Dawa-Background") == "1"
}
