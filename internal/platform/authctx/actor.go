// Package authctx carries the authenticated caller through the request context.
//
// It exists so that a module handler can ask "who is calling?" without importing
// the identity module — modules must not depend on each other (see ADR 0002 and
// the depguard rules), and platform packages are the shared ground they are all
// allowed to reach.
//
// The rule this package makes enforceable: the acting user is whoever
// authentication proved them to be, never whoever the request said they were.
// Handlers previously read `?user_id=` from the query string, which let any
// caller act as any user simply by changing a number.
package authctx

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type ctxKey int

const ctxKeyActor ctxKey = iota

// Actor is the authenticated caller.
type Actor struct {
	UserID         int64
	OrganizationID int64
	OrgID          int64  // Alias for OrganizationID
	OrgType        string // "customer" | "vendor" | "" for a user with no organization or staff-only
	OrgStatus      string // pending | approved | rejected | suspended
	BranchID       *int64 // active/selected or bound branch
	BoundBranchID  *int64 // non-nil only when the member is strictly bound to one branch in org.members
	Role           string // platform role
	Permissions    []string
	IsStaff        bool
	Email          string
	Name           string
	AvatarURL      string
	// Scope names the dashboard this actor's permissions belong to. It decides
	// which sidebar is rendered and which permissions a role editor may offer,
	// and it is set by authentication from the resolved grant — never from the
	// request.
	Scope rbac.Scope
	// IsOwner marks the platform super admin or a company owner: the holder of
	// everything within their own scope.
	IsOwner bool

	// perms is the matched form of Permissions, built lazily. Actor is copied
	// by value through the context, so this is rebuilt per copy rather than
	// shared — the slice is short and the map build is cheaper than the linear
	// scan it replaces once a handler asks more than twice.
	perms *rbac.Set
}

// Grants replaces the actor's holding. Only authentication middleware and the
// tests that stand in for it should call this.
func (a *Actor) Grants(keys []string) {
	a.Permissions = keys
	set := rbac.NewSet(keys)
	a.perms = &set
}

// set returns the matcher for this actor's holding.
func (a Actor) set() rbac.Set {
	if a.perms != nil {
		return *a.perms
	}
	return rbac.NewSet(a.Permissions)
}

// IsPlatformAdmin reports whether the actor has super_admin or admin role.
func (a Actor) IsPlatformAdmin() bool {
	return a.Role == "super_admin" || a.Role == "admin"
}

// IsSuperAdmin reports whether the actor has super_admin role.
func (a Actor) IsSuperAdmin() bool {
	return a.Role == "super_admin"
}

// DashboardScope reports which dashboard this actor belongs to.
//
// Scope is set by authentication from the resolved grant. The OrgType fallback
// covers an actor built without a resolver — tests, and the degraded path when
// the database is unreachable — and it goes through rbac.TenantScopeFor so a
// legacy type spelling cannot produce a different answer here than elsewhere.
func (a Actor) DashboardScope() rbac.Scope {
	if a.IsStaff {
		return rbac.ScopeAdmin
	}
	if a.Scope != "" {
		return a.Scope
	}
	if scope, ok := rbac.TenantScopeFor(a.OrgType); ok {
		return scope
	}
	return ""
}

// IsCustomer reports whether the actor belongs to a customer (صيدلية) tenant.
// An organization member with no resolved type is treated as a customer so a
// pending organization cannot reach vendor surfaces by omission.
func (a Actor) IsCustomer() bool {
	if a.IsStaff || a.IsJobSeeker() {
		return false
	}
	return a.DashboardScope() != rbac.ScopeVendor
}

// IsVendor reports whether the actor belongs to a vendor (مورّد) tenant.
func (a Actor) IsVendor() bool {
	return !a.IsStaff && a.DashboardScope() == rbac.ScopeVendor
}

// IsOrgApproved reports whether the actor's organization is approved, active, or verified,
// or if the actor is platform staff.
func (a Actor) IsOrgApproved() bool {
	if a.IsStaff {
		return true
	}
	switch a.OrgStatus {
	case "approved", "active", "verified":
		return true
	default:
		return false
	}
}

// IsJobSeeker reports whether the actor has the job_seeker platform role and has not been hired into an organization.
func (a Actor) IsJobSeeker() bool {
	return a.Role == "job_seeker" && a.OrganizationID == 0
}

// DisplayName returns a user-friendly name to display in the navbar.
func (a Actor) DisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.Email != "" {
		for i, r := range a.Email {
			if r == '@' {
				return a.Email[:i]
			}
		}
		return a.Email
	}
	if a.IsStaff {
		return "مدير المنصة"
	}
	if a.Role == "job_seeker" {
		return "باحث عن عمل"
	}
	if a.IsVendor() {
		return "مورّد أدوية"
	}
	return "صيدلي معتمد"
}

// Can reports whether the actor holds a permission.
//
// Matching is hierarchical: a holder of "catalog.*" satisfies
// "catalog.product.view", and a holder of "*" satisfies everything. The old
// implementation compared strings exactly, so the wildcard grant that
// RequirePagePermission checked for separately was invisible here — two
// callers asking the same question got different answers.
func (a Actor) Can(permission string) bool {
	return a.set().Has(permission)
}

// CanAny reports whether the actor holds at least one of the permissions. An
// empty list is an ungated requirement and passes.
func (a Actor) CanAny(permissions ...string) bool {
	return a.set().HasAny(permissions...)
}

// CanAll reports whether the actor holds every one of the permissions.
func (a Actor) CanAll(permissions ...string) bool {
	return a.set().HasAll(permissions...)
}

// WithActor binds the authenticated caller to the context. Only authentication
// middleware should call this.
func WithActor(ctx context.Context, a Actor) context.Context {
	if a.OrgID <= 0 && a.OrganizationID > 0 {
		a.OrgID = a.OrganizationID
	} else if a.OrganizationID <= 0 && a.OrgID > 0 {
		a.OrganizationID = a.OrgID
	}
	return context.WithValue(ctx, ctxKeyActor, a)
}

// ContextWithActor is an alias for WithActor for callers expecting the standard ContextWith... naming convention.
func ContextWithActor(ctx context.Context, a Actor) context.Context {
	return WithActor(ctx, a)
}

// BranchOption names one of the customer's own branches in the shell selector.
type BranchOption struct {
	ID   int64
	Name string // localized display name
}

// BuyingBranch is the branch the customer is buying for: one of their own
// branches, chosen in the customer shell and persisted per browser. The
// option list always comes from the database; only the selection rides the
// cookie, and handlers resolve coordinates from the branch record.
type BuyingBranch struct {
	Branches []BranchOption
	Active   *int64
	IsLocked bool
}

type ctxKeyBuyingBranch struct{}

// WithBuyingBranch binds the customer's branch selection to the context.
func WithBuyingBranch(ctx context.Context, b BuyingBranch) context.Context {
	return context.WithValue(ctx, ctxKeyBuyingBranch{}, b)
}

// BuyingBranchFrom returns the branch selection bound for this request.
func BuyingBranchFrom(ctx context.Context) (BuyingBranch, bool) {
	b, ok := ctx.Value(ctxKeyBuyingBranch{}).(BuyingBranch)
	return b, ok
}

// From returns the authenticated caller, if any.
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKeyActor).(Actor)
	if ok && a.OrgID == 0 && a.OrganizationID > 0 {
		a.OrgID = a.OrganizationID
	}
	if ok && a.OrganizationID == 0 && a.OrgID > 0 {
		a.OrganizationID = a.OrgID
	}
	return a, ok && a.UserID > 0
}

// FromContext returns the authenticated caller from context, or an empty Actor.
func FromContext(ctx context.Context) Actor {
	a, _ := From(ctx)
	return a
}

// UserID returns the authenticated user id, or an Unauthorized error.
//
// Handlers use this instead of parsing an identifier from the request. It
// cannot be spoofed, because nothing outside authentication middleware can put
// an actor into the context.
func UserID(ctx context.Context) (int64, error) {
	a, ok := From(ctx)
	if !ok {
		return 0, apperr.Unauthorized()
	}
	return a.UserID, nil
}

// SameUserOrForbidden guards access to another user's data.
//
// Pass the id the request is trying to reach. It is permitted only when it is
// the caller's own id, or when the caller holds the override permission — which
// is how support staff legitimately read someone else's record.
func SameUserOrForbidden(ctx context.Context, targetUserID int64, overridePermission string) error {
	a, ok := From(ctx)
	if !ok {
		return apperr.Unauthorized()
	}
	if a.UserID == targetUserID {
		return nil
	}
	if overridePermission != "" && a.Can(overridePermission) {
		return nil
	}
	return apperr.Forbidden("actor.not_owner",
		"You do not have access to another user's data.")
}

// SameOrgOrForbidden guards access to another tenant's data.
//
// Pass the organization id taken from the route. Authentication establishes who
// is calling; it says nothing about which tenant they may act on. A handler that
// reads {id} from the URL and passes it straight to a service lets any logged-in
// user address any organization, which is how PUT /org/organizations/{id} came
// to accept another tenant's id.
//
// This matters more here than it looks: the application connects to PostgreSQL
// as a superuser, so row-level security is inert and this check is the only
// thing enforcing the boundary.
func SameOrgOrForbidden(ctx context.Context, targetOrgID int64, overridePermission string) error {
	a, ok := From(ctx)
	if !ok {
		return apperr.Unauthorized()
	}
	if a.OrganizationID == targetOrgID {
		return nil
	}
	// Platform staff legitimately act across tenants; they hold the permission.
	if overridePermission != "" && a.Can(overridePermission) {
		return nil
	}
	return apperr.Forbidden("actor.wrong_tenant",
		"You do not have access to another organization's data.")
}
