package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type erpPageRepo struct {
	erp.Repository
	keys   map[int64]erp.NewKey
	next   int64
	audits int
}

func (r *erpPageRepo) EnsureIntegration(_ context.Context, org int64, p string, _ int64, _ erp.AuditRecord) (erp.Integration, error) {
	return erp.Integration{ID: org, OrganizationID: org, Provider: p, Status: "active"}, nil
}
func (r *erpPageRepo) OrgStatusType(_ context.Context, org int64) (string, string, error) {
	if org == 2 {
		return "approved", "customer", nil
	}
	return "approved", "vendor", nil
}
func (r *erpPageRepo) WarehouseInOrg(context.Context, int64, int64) (bool, error) { return true, nil }
func (r *erpPageRepo) BranchInOrg(context.Context, int64, int64) (bool, error)    { return true, nil }
func (r *erpPageRepo) KeyTargets(context.Context, int64) ([]erp.KeyTarget, []erp.KeyTarget, error) {
	return []erp.KeyTarget{{ID: 7, Name: map[string]string{"ar": "المخزن الرئيسي", "en": "Main warehouse"}}},
		[]erp.KeyTarget{{ID: 9, Name: map[string]string{"ar": "الفرع الرئيسي", "en": "Main branch"}}}, nil
}
func (r *erpPageRepo) ActiveMember(context.Context, int64, int64) (bool, error) { return true, nil }
func (r *erpPageRepo) InsertKey(_ context.Context, org int64, k erp.NewKey, _ erp.AuditRecord) (erp.APIKey, error) {
	r.next++
	k.ID = r.next
	k.OrganizationID = org
	k.Status = "active"
	r.keys[k.ID] = k
	r.audits++
	return k.APIKey, nil
}
func (r *erpPageRepo) GetKey(_ context.Context, org, id int64) (erp.APIKey, error) {
	k, ok := r.keys[id]
	if !ok || k.OrganizationID != org {
		return erp.APIKey{}, erp.ErrNotFound
	}
	return k.APIKey, nil
}
func (r *erpPageRepo) ListKeys(_ context.Context, org int64) ([]erp.APIKey, error) {
	var out []erp.APIKey
	for _, k := range r.keys {
		if k.OrganizationID == org {
			out = append(out, k.APIKey)
		}
	}
	return out, nil
}
func (r *erpPageRepo) ListKeysAsStaff(context.Context) ([]erp.StaffKeyRow, error) {
	var out []erp.StaffKeyRow
	for _, k := range r.keys {
		out = append(out, erp.StaffKeyRow{Key: k.APIKey, OrganizationName: map[string]string{"ar": "منشأة", "en": "Org"}})
	}
	return out, nil
}
func (r *erpPageRepo) SetKeyExpiry(_ context.Context, org, id int64, at time.Time, _ erp.AuditRecord) error {
	k, ok := r.keys[id]
	if !ok || k.OrganizationID != org {
		return erp.ErrNotFound
	}
	k.ExpiresAt = &at
	r.keys[id] = k
	r.audits++
	return nil
}
func (r *erpPageRepo) SetKeyRateLimit(_ context.Context, org, id int64, rate int, _ erp.AuditRecord) error {
	k, ok := r.keys[id]
	if !ok || k.OrganizationID != org {
		return erp.ErrNotFound
	}
	k.RateLimitPerMinute = rate
	r.keys[id] = k
	r.audits++
	return nil
}
func (r *erpPageRepo) RevokeKey(_ context.Context, org, id int64, reason string, _ int64, _ erp.AuditRecord) error {
	k, ok := r.keys[id]
	if !ok || k.OrganizationID != org {
		return erp.ErrNotFound
	}
	k.Status = "revoked"
	r.keys[id] = k
	r.audits++
	return nil
}
func (r *erpPageRepo) RevokeKeyAsStaff(ctx context.Context, id int64, reason string, by int64, a erp.AuditRecord) error {
	k, ok := r.keys[id]
	if !ok {
		return erp.ErrNotFound
	}
	return r.RevokeKey(ctx, k.OrganizationID, id, reason, by, a)
}
func erpPageFixture() (*UIHandler, *erpPageRepo) {
	repo := &erpPageRepo{keys: map[int64]erp.NewKey{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	h.SetERP(erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: []byte("erp test pepper"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, log))
	return h, repo
}
func erpPageActor(org int64, owner bool) authctx.Actor {
	kind := "vendor"
	if org == 2 {
		kind = "customer"
	}
	return authctx.Actor{UserID: 5, OrganizationID: org, OrgID: org, OrgType: kind, OrgStatus: "approved", IsOwner: owner, Permissions: []string{kind + ".*"}}
}
func erpPageRequest(actor authctx.Actor, path string, values url.Values, id string) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := authctx.WithActor(req.Context(), actor)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rc))
}
func validERPForm() url.Values {
	return url.Values{"name": {"Key"}, "warehouse_id": {"10"}, "branch_id": {"11"}, "rate_limit": {"600"}}
}
func TestErpCreateOwnerAndOnce(t *testing.T) {
	h, repo := erpPageFixture()
	nonOwner := erpPageActor(1, false)
	v := validERPForm()
	v.Set("is_owner", "true")
	v.Set("actorIsOwner", "true")
	w := httptest.NewRecorder()
	h.ErpKeyCreate(w, erpPageRequest(nonOwner, "/vendor/integrations/keys", v, ""))
	if len(repo.keys) != 0 || w.Code == 200 {
		t.Fatal("non-owner created a key")
	}
	owner := erpPageActor(1, true)
	w = httptest.NewRecorder()
	h.ErpKeyCreate(w, erpPageRequest(owner, "/vendor/integrations/keys", validERPForm(), ""))
	if len(repo.keys) != 1 || w.Code != 200 {
		t.Fatalf("create status=%d keys=%d", w.Code, len(repo.keys))
	}
	body := w.Body.String()
	if !strings.Contains(body, "d24_test_") {
		t.Fatal("create response lacks full key")
	}
	var prefix string
	for _, k := range repo.keys {
		prefix = k.KeyPrefix
	}
	page := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/vendor/integrations", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), owner))
	h.ErpKeysPage(page, req)
	if !strings.Contains(page.Body.String(), prefix) || strings.Contains(page.Body.String(), "d24_test_") {
		t.Error("reload must show prefix but not full key")
	}
}
func TestErpOwnerMutationsAndLimits(t *testing.T) {
	h, repo := erpPageFixture()
	repo.keys[7] = erp.NewKey{APIKey: erp.APIKey{ID: 7, OrganizationID: 1, IntegrationID: 1, Name: "Key", KeyPrefix: "abcdefgh", Scopes: []string{erp.ScopeOrdersRead}, WarehouseID: 10, BranchID: 11, RateLimitPerMinute: 600, MaxBatchItems: 50000, Status: "active"}}
	repo.next = 7
	for _, method := range []func(http.ResponseWriter, *http.Request){h.ErpKeyRotate, h.ErpKeyRevoke, h.ErpKeyLimits} {
		before := repo.audits
		v := url.Values{"rate_limit": {"700"}, "grace_hours": {"1"}, "is_owner": {"true"}, "actorIsOwner": {"true"}}
		w := httptest.NewRecorder()
		method(w, erpPageRequest(erpPageActor(1, false), "/vendor/integrations/keys/7", v, "7"))
		if repo.audits != before || len(repo.keys) != 1 {
			t.Error("non-owner mutation succeeded")
		}
	}
	for _, value := range []string{"9", "6001"} {
		w := httptest.NewRecorder()
		h.ErpKeyLimits(w, erpPageRequest(erpPageActor(1, true), "/vendor/integrations/keys/7", url.Values{"rate_limit": {value}}, "7"))
		if repo.keys[7].RateLimitPerMinute != 600 {
			t.Errorf("invalid limit %s saved", value)
		}
	}
	w := httptest.NewRecorder()
	h.ErpKeyLimits(w, erpPageRequest(erpPageActor(1, true), "/vendor/integrations/keys/7", url.Values{"rate_limit": {"700"}}, "7"))
	if repo.keys[7].RateLimitPerMinute != 700 {
		t.Error("owner limit not saved")
	}
}
func TestErpFormRejectsUnknownScope(t *testing.T) {
	v := validERPForm()
	v.Set("scopes", "unknown.scope")
	req := erpPageRequest(erpPageActor(1, true), "/", v, "")
	_, errs := parseCreateKeyForm(req, true)
	if len(errs) == 0 {
		t.Error("unknown scope accepted")
	}
}
func TestErpStaffAndPharmacyPages(t *testing.T) {
	h, repo := erpPageFixture()
	repo.keys[1] = erp.NewKey{APIKey: erp.APIKey{ID: 1, OrganizationID: 1, Name: "Vendor key", KeyPrefix: "aaaaaaaa", Status: "active"}}
	repo.keys[2] = erp.NewKey{APIKey: erp.APIKey{ID: 2, OrganizationID: 2, Name: "Pharmacy key", KeyPrefix: "bbbbbbbb", Status: "active"}}
	pharmacy := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/customer/integrations", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), erpPageActor(2, true)))
	h.ErpKeysPage(pharmacy, req)
	if !strings.Contains(pharmacy.Body.String(), "bbbbbbbb") || strings.Contains(pharmacy.Body.String(), "aaaaaaaa") {
		t.Error("pharmacy key list is not tenant isolated")
	}
	staff := erpPageActor(1, true)
	staff.IsStaff = true
	staff.Role = "super_admin"
	req = httptest.NewRequest("GET", "/admin/integrations", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), staff))
	w := httptest.NewRecorder()
	h.ErpAdminKeysPage(w, req)
	if !strings.Contains(w.Body.String(), "aaaaaaaa") || !strings.Contains(w.Body.String(), "bbbbbbbb") {
		t.Error("staff list missing keys")
	}
	w = httptest.NewRecorder()
	h.ErpAdminKeyRevoke(w, erpPageRequest(staff, "/admin/integrations/keys/2/revoke", url.Values{"reason": {"test"}}, "2"))
	if repo.keys[2].Status != "revoked" {
		t.Error("staff revoke failed")
	}
}

// The "new key" button opens erp-create-key-modal; the page must carry that
// dialog (it was referenced but never rendered), with the organization's own
// warehouses and branches as choices. A non-owner gets neither.
func TestErpKeysPageRendersCreateDialogForOwner(t *testing.T) {
	h, _ := erpPageFixture()
	get := func(owner bool) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/vendor/integrations", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), erpPageActor(1, owner)))
		h.ErpKeysPage(w, req)
		return w.Body.String()
	}
	body := get(true)
	for _, want := range []string{`id="erp-create-key-modal"`, `action="/vendor/integrations/keys"`,
		`name="warehouse_id"`, `<option value="7">`, `name="branch_id"`, `<option value="9">`} {
		if !strings.Contains(body, want) {
			t.Errorf("owner page lacks %s", want)
		}
	}
	if strings.Contains(get(false), `id="erp-create-key-modal"`) {
		t.Error("non-owner page renders the create dialog")
	}
}
