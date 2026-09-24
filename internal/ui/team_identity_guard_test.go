package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type membershipRepo struct {
	org.Repository
	orgs map[int64][]int64
}

func (m membershipRepo) MemberOrganizations(_ context.Context, userID int64) ([]int64, error) {
	return m.orgs[userID], nil
}

// A supplier must not adopt, or rewrite the profile of, an account that is
// platform staff or already works for another company.
func TestAccountManagedByOrg(t *testing.T) {
	repo := membershipRepo{orgs: map[int64][]int64{
		1: nil,      // free account
		2: {10},     // already in this company
		3: {20},     // works for a competitor
		4: {10, 20}, // shared
		5: nil,      // staff
	}}
	h := &UIHandler{orgSvc: org.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))}
	ctx := context.Background()

	cases := []struct {
		user *identity.User
		want bool
	}{
		{&identity.User{ID: 1, Role: "user"}, true},
		{&identity.User{ID: 2, Role: "user"}, true},
		{&identity.User{ID: 3, Role: "user"}, false},
		{&identity.User{ID: 4, Role: "pharmacy"}, false},
		{&identity.User{ID: 5, Role: identity.RoleSuperAdmin}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := h.accountManagedByOrg(ctx, tc.user, 10); got != tc.want {
			t.Errorf("accountManagedByOrg(%+v) = %v; want %v", tc.user, got, tc.want)
		}
	}
}

type rolesRepo struct {
	org.Repository
	roles []*org.Role
}

func (r rolesRepo) ListRoles(context.Context, int64) ([]*org.Role, error) { return r.roles, nil }

// A team manager cannot hand out a role carrying permissions they lack.
func TestResolveTeamRoleCeiling(t *testing.T) {
	roles := []*org.Role{
		{ID: 1, Key: "org_owner", IsOwner: true, Permissions: []string{"vendor.wallet.manage", "vendor.team.manage"}},
		{ID: 2, Key: "org_manager", Permissions: []string{"vendor.wallet.manage", "vendor.team.manage", "vendor.order.view"}},
		{ID: 3, Key: "org_employee", Permissions: []string{"vendor.order.view"}},
	}
	h := &UIHandler{orgSvc: org.NewService(rolesRepo{roles: roles}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	manager := authctx.Actor{UserID: 9, OrganizationID: 10}
	manager.Grants([]string{"vendor.team.manage", "vendor.order.view"})
	ctx := authctx.WithActor(context.Background(), manager)

	if id, key := h.resolveTeamRole(ctx, 10, "2", ""); id == 2 {
		t.Errorf("team manager assigned org_manager (%d %s) without holding its permissions", id, key)
	}
	if id, _ := h.resolveTeamRole(ctx, 10, "3", ""); id != 3 {
		t.Errorf("team manager could not assign a role within their authority: got %d", id)
	}

	owner := authctx.Actor{UserID: 1, OrganizationID: 10, IsOwner: true}
	if id, _ := h.resolveTeamRole(authctx.WithActor(context.Background(), owner), 10, "2", ""); id != 2 {
		t.Errorf("owner could not assign org_manager: got %d", id)
	}
}
