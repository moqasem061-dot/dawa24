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

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	orgHttp "github.com/moqasem061-dot/dawa24/internal/modules/org/http"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/org/organizations", nil)
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
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, body)
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
