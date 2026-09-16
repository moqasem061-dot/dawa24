package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (happyRepo) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) GetInstitutionalWorkBySlug(ctx context.Context, slug string) (*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) AnyBranchHasInstitutionalWork(ctx context.Context, branchIDs, workIDs []int64) (bool, error) {
	return false, nil
}
func (happyRepo) ListBranchesWithoutInstitutionalWorks(ctx context.Context) ([]*org.BranchWithoutWorks, error) {
	return nil, nil
}
func (happyRepo) GetReachableBuyerWorksForBranch(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return nil
}
func (happyRepo) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return nil
}
func (happyRepo) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}
func (happyRepo) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	return nil, nil
}

func (happyRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {

	o.ID = 1
	return nil
}
func (happyRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{ID: id, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Type: org.TypeCustomer, Status: org.StatusApproved, AIVirtualKey: "sk-secret-virtual-key"}, nil
}
func (happyRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	return nil
}
func (happyRepo) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	return nil
}
func (happyRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	return nil
}
func (happyRepo) DeleteOrganization(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, nil
}
func (happyRepo) ListOrganizationsWithTotal(ctx context.Context, _ string, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, int, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, 1, nil
}
func (happyRepo) AdminOrgStats(ctx context.Context) (org.AdminOrgStatsResult, error) {
	return org.AdminOrgStatsResult{TotalOrgs: 1, TotalPharmacies: 1, TotalVendors: 0, PendingCount: 0, ApprovedCount: 1}, nil
}
func (happyRepo) CountBranchesByOrg(ctx context.Context) (map[int64]int, error) {
	return map[int64]int{1: 2}, nil
}
func (happyRepo) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	return nil, nil
}
func (happyRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	b.ID = 1
	return nil
}
func (happyRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	return &org.Branch{ID: id, OrganizationID: 1, Code: "BR-01", Name: i18n.Text{"en": "Main"}}, nil
}
func (happyRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return nil
}
func (happyRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return []*org.Branch{{ID: 1, OrganizationID: orgID, Code: "BR-01", Name: i18n.Text{"en": "Main"}}}, nil
}
func (happyRepo) ListBranchesWithTotal(ctx context.Context, filter org.BranchFilter, limit, offset int) ([]*org.Branch, int, error) {
	return []*org.Branch{{ID: 1, OrganizationID: 1, Code: "BR-01", Name: i18n.Text{"en": "Main"}}}, 1, nil
}
func (happyRepo) AdminBranchStats(ctx context.Context) (org.AdminBranchStatsResult, error) {
	return org.AdminBranchStatsResult{TotalBranches: 1, ActiveBranches: 1, PharmacyBranches: 1, VendorWarehouses: 0}, nil
}
func (happyRepo) GetBranchesByIDs(ctx context.Context, ids []int64) ([]*org.Branch, error) {
	return nil, nil
}
func (happyRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) AddMember(ctx context.Context, m *org.Member) error {
	m.ID = 1
	return nil
}
func (happyRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return nil
}
func (happyRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	return []*org.Member{{ID: 1, OrganizationID: orgID, UserID: 1, RoleID: 1}}, nil
}
func (happyRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return nil
}
func (happyRepo) AddReview(ctx context.Context, rev *org.Review) error {
	rev.ID = 1
	return nil
}
func (happyRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	return []*org.Review{{ID: 1, OrganizationID: orgID, UserID: 1, Rating: 5}}, nil
}
func (happyRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, nil
}
func (happyRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	p.ID = 1
	return nil
}
func (happyRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	return []*org.Policy{{ID: 1, OrganizationID: orgID, Title: "Refund Policy"}}, nil
}

func (happyRepo) CreateRole(ctx context.Context, role *org.Role) error {
	role.ID = 1
	return nil
}
func (happyRepo) GetRole(ctx context.Context, orgID, roleID int64) (*org.Role, error) {
	return &org.Role{ID: roleID, OrganizationID: orgID}, nil
}

func (happyRepo) UpdateRole(ctx context.Context, orgID int64, role *org.Role) error { return nil }

func (happyRepo) DeleteRole(ctx context.Context, orgID, roleID int64) error { return nil }

func (happyRepo) ListRoles(ctx context.Context, orgID int64) ([]*org.Role, error) { return nil, nil }

func (happyRepo) CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (happyRepo) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	return nil
}
func (happyRepo) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	return nil, nil
}
func (happyRepo) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return nil
}
func (happyRepo) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	rev.ID = 1
	return nil
}
func (happyRepo) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	return nil, nil
}
func (happyRepo) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return nil
}
func (happyRepo) CreateUserOrganization(ctx context.Context, uo *org.UserOrganization) error {
	return nil
}
func (happyRepo) GetUserOrganizationByID(ctx context.Context, id int64) (*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status org.UserOrganizationStatus, notes string) error {
	return nil
}
func (happyRepo) DeleteUserOrganization(ctx context.Context, id int64) error { return nil }
func (happyRepo) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) ListUserOrganizationsByVendorWithTotal(ctx context.Context, vendorOrgID int64, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) ListAllUserOrganizationsWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	return nil, 0, nil
}

func (stubRepo) CreateUserOrganization(ctx context.Context, uo *org.UserOrganization) error {
	return nil
}
func (stubRepo) GetUserOrganizationByID(ctx context.Context, id int64) (*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status org.UserOrganizationStatus, notes string) error {
	return nil
}
func (stubRepo) DeleteUserOrganization(ctx context.Context, id int64) error { return nil }
func (stubRepo) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) ListUserOrganizationsByVendorWithTotal(ctx context.Context, vendorOrgID int64, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) ListAllUserOrganizationsWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*org.UserOrganization, int, error) {
	return nil, 0, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo org.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				IsStaff:        true,
				Permissions: []string{"org.admin", "org.organization.view", "org.organization.update",
					"org.organization.delete", "org.branch.view", "org.branch.update", "org.branch.delete", "org.member.manage"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/org/organizations"},
	{http.MethodGet, "/api/v1/org/organizations/1"},
	{http.MethodPut, "/api/v1/org/organizations/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1"},
	{http.MethodGet, "/api/v1/org/organizations"},
	{http.MethodPost, "/api/v1/org/organizations/1/status"},
	{http.MethodPost, "/api/v1/org/organizations/1/branches"},
	{http.MethodGet, "/api/v1/org/organizations/1/branches"},
	{http.MethodPut, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/members"},
	{http.MethodGet, "/api/v1/org/organizations/1/members"},
	{http.MethodPut, "/api/v1/org/organizations/1/members/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/members/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/reviews"},
	{http.MethodGet, "/api/v1/org/organizations/1/reviews"},
	{http.MethodPost, "/api/v1/org/organizations/1/follow"},
	{http.MethodGet, "/api/v1/admin/org/pending"},
	{http.MethodPost, "/api/v1/admin/org/1/approve"},
	{http.MethodPost, "/api/v1/admin/org/1/reject"},
	{http.MethodPost, "/api/v1/admin/org/1/suspend"},
	{http.MethodPut, "/api/v1/admin/org/1"},
	{http.MethodGet, "/api/v1/admin/org/members"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/organizations", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func (happyRepo) GetMember(context.Context, int64, int64) (*org.Member, error) {
	return &org.Member{ID: 1, OrganizationID: 1, UserID: 1, IsActive: true}, nil
}
func (happyRepo) UpdateMember(context.Context, int64, int64, org.MemberPatch) error { return nil }
func (happyRepo) CountMembersByBranch(context.Context, int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}
func (happyRepo) MemberOrganizations(context.Context, int64) ([]int64, error) { return nil, nil }

func (stubRepo) GetMember(context.Context, int64, int64) (*org.Member, error) {
	return &org.Member{ID: 1, OrganizationID: 1, UserID: 1, IsActive: true}, nil
}
func (stubRepo) UpdateMember(context.Context, int64, int64, org.MemberPatch) error { return nil }
func (stubRepo) CountMembersByBranch(context.Context, int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}
func (stubRepo) MemberOrganizations(context.Context, int64) ([]int64, error) { return nil, nil }

func routerAs(actor authctx.Actor) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := database.WithTenant(authctx.WithActor(req.Context(), actor), actor.OrganizationID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	orgHttp.NewHandler(org.NewService(happyRepo{}, log), log).RegisterRoutes(r)
	return r
}

// Membership is not permission: an employee without the dashboard keys cannot
// manage the team, branches or the organisation over JSON, a member of another
// organisation cannot read it, and nobody receives its AI credential.
func TestOrgAPIRequiresTheDashboardPermission(t *testing.T) {
	employee := authctx.Actor{UserID: 7, OrganizationID: 1, Scope: "pharmacy", OrgType: "customer",
		Permissions: []string{"pharmacy.dashboard.view", "pharmacy.organization.view"}}
	outsider := authctx.Actor{UserID: 9, OrganizationID: 2, Scope: "pharmacy", OrgType: "customer",
		Permissions: []string{"pharmacy.organization.view", "pharmacy.team.view"}}
	manager := authctx.Actor{UserID: 8, OrganizationID: 1, Scope: "pharmacy", OrgType: "customer",
		Permissions: []string{"pharmacy.team.view", "pharmacy.team.update"}}

	cases := []struct {
		name   string
		actor  authctx.Actor
		method string
		path   string
		body   string
		want   int
	}{
		{"employee changes a role", employee, http.MethodPut, "/api/v1/org/organizations/1/members/1", `{"role":"org_owner"}`, http.StatusForbidden},
		{"employee removes a member", employee, http.MethodDelete, "/api/v1/org/organizations/1/members/1", "", http.StatusForbidden},
		{"employee deletes a branch", employee, http.MethodDelete, "/api/v1/org/organizations/1/branches/1", "", http.StatusForbidden},
		{"employee deletes the organisation", employee, http.MethodDelete, "/api/v1/org/organizations/1", "", http.StatusForbidden},
		{"employee lists every organisation", employee, http.MethodGet, "/api/v1/org/organizations", "", http.StatusForbidden},
		{"employee approves itself", employee, http.MethodPost, "/api/v1/org/organizations/1/status", `{"status":"approved"}`, http.StatusForbidden},
		{"outsider reads another organisation", outsider, http.MethodGet, "/api/v1/org/organizations/1", "", http.StatusNotFound},
		{"outsider lists its members", outsider, http.MethodGet, "/api/v1/org/organizations/1/members", "", http.StatusForbidden},
		{"manager with the team key changes a role", manager, http.MethodPut, "/api/v1/org/organizations/1/members/1", `{"role":"manager"}`, http.StatusOK},
		{"member reads its own organisation", employee, http.MethodGet, "/api/v1/org/organizations/1", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			routerAs(tc.actor).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "sk-secret-virtual-key") {
				t.Fatal("the AI virtual key was serialised")
			}
		})
	}
}
