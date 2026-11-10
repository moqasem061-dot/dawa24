package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// erpActivityRepo serves the activity reads from memory. The order counts stay
// unread because the fixture has no integration id.
type erpActivityRepo struct {
	*erpPageRepo
	counts  erp.ActivityCounts
	calls   []erp.CallRow
	batches []erp.BatchRow
	imports []erp.OrderImportRow
}

func (r *erpActivityRepo) ActivityCounts(context.Context, int64) (erp.ActivityCounts, error) {
	return r.counts, nil
}

func (r *erpActivityRepo) ListCalls(context.Context, int64, erp.CallFilter) ([]erp.CallRow, error) {
	return r.calls, nil
}

func (r *erpActivityRepo) ListBatches(context.Context, int64, int64, int) ([]erp.BatchRow, error) {
	return r.batches, nil
}

func (r *erpActivityRepo) ListActiveBatches(context.Context, int64, int) ([]erp.BatchRow, error) {
	var out []erp.BatchRow
	for _, b := range r.batches {
		if b.State == "queued" || b.State == "processing" {
			out = append(out, b)
		}
	}
	return out, nil
}

// GetBatchByID and GetBatchByPublicID read only the organization's own batches,
// as the SQL does; anything else is not found.
func (r *erpActivityRepo) GetBatchByID(_ context.Context, org, id int64) (erp.SyncBatch, error) {
	for _, b := range r.batches {
		if b.OrganizationID == org && b.ID == id {
			return b.SyncBatch, nil
		}
	}
	return erp.SyncBatch{}, erp.ErrNotFound
}

func (r *erpActivityRepo) GetBatchByPublicID(ctx context.Context, org int64, syncID string) (erp.SyncBatch, error) {
	id, err := strconv.ParseInt(syncID, 10, 64)
	if err != nil {
		return erp.SyncBatch{}, erp.ErrNotFound
	}
	return r.GetBatchByID(ctx, org, id)
}

func (r *erpActivityRepo) ListOrderImports(context.Context, int64, int) ([]erp.OrderImportRow, error) {
	return r.imports, nil
}

func erpActivityFixture(t *testing.T) (*UIHandler, *erpActivityRepo) {
	t.Helper()
	h, pageRepo := erpPageFixture()
	repo := &erpActivityRepo{erpPageRepo: pageRepo}
	h.SetERP(erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: []byte("erp test pepper"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000}, h.log))
	return h, repo
}

func erpActivityGET(t *testing.T, h *UIHandler, path string, actor authctx.Actor, handler func(http.ResponseWriter, *http.Request)) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, w.Code)
	}
	return w.Body.String()
}

func TestActivityPageRendersEveryVendorTab(t *testing.T) {
	h, repo := erpActivityFixture(t)
	repo.counts = erp.ActivityCounts{IntegrationStatus: "active", ActiveKeys: 2}
	repo.calls = []erp.CallRow{{ID: 9, KeyPrefix: "aaaaaaaa", Method: "GET", Route: "/products/sync/{syncId}", Status: 200, RequestID: "abcdef0123456789", CreatedAt: time.Now()}}
	repo.batches = []erp.BatchRow{{SyncBatch: erp.SyncBatch{ID: 4821, PublicID: "4821", State: "processing", ItemCount: 10, CreatedCount: 4, AcceptedAt: time.Now()}, KeyName: "Main"}}
	owner := erpPageActor(1, true)
	for _, tab := range []string{"overview", "syncs", "calls", "orders"} {
		body := erpActivityGET(t, h, "/vendor/integrations/activity?tab="+tab, owner, h.ErpActivityPage)
		if !strings.Contains(body, "سجل التكامل") {
			t.Fatalf("tab %s: title missing", tab)
		}
	}
	syncs := erpActivityGET(t, h, "/vendor/integrations/activity?tab=syncs", owner, h.ErpActivityPage)
	if !strings.Contains(syncs, "#4821") {
		t.Fatal("syncs tab: batch 4821 missing")
	}
	calls := erpActivityGET(t, h, "/vendor/integrations/activity?tab=calls", owner, h.ErpActivityPage)
	if !strings.Contains(calls, "abcdef0123456789") {
		t.Fatal("calls tab: request id missing")
	}
}

func TestActivityPharmacyHasNoSyncsTab(t *testing.T) {
	h, repo := erpActivityFixture(t)
	repo.counts = erp.ActivityCounts{IntegrationStatus: "active"}
	pharmacy := erpPageActor(2, false)
	body := erpActivityGET(t, h, "/customer/integrations/activity", pharmacy, h.ErpActivityPage)
	if strings.Contains(body, "tab=syncs") {
		t.Fatal("pharmacy page links to the syncs tab")
	}
	body = erpActivityGET(t, h, "/customer/integrations/activity?tab=syncs", pharmacy, h.ErpActivityPage)
	if strings.Contains(body, "tab=syncs") || !strings.Contains(body, "tab=calls") {
		t.Fatal("pharmacy tab=syncs must read as the overview")
	}
}

func TestActivityLiveFragmentPollsOnlyWhileBatchActive(t *testing.T) {
	h, repo := erpActivityFixture(t)
	owner := erpPageActor(1, true)
	live := func() string {
		return erpActivityGET(t, h, "/vendor/integrations/activity/live", owner, h.ErpActivityLive)
	}
	repo.batches = []erp.BatchRow{{SyncBatch: erp.SyncBatch{ID: 1, PublicID: "1", State: "completed", ItemCount: 2}}}
	if body := live(); strings.Contains(body, "hx-trigger") {
		t.Fatal("no active batch: the panel must not poll")
	}
	repo.batches = []erp.BatchRow{{SyncBatch: erp.SyncBatch{ID: 2, PublicID: "2", State: "processing", ItemCount: 2, ChunkCount: 2}}}
	if body := live(); !strings.Contains(body, `hx-trigger="every 5s"`) {
		t.Fatal("active batch: the panel must poll every 5s")
	}
}

func TestActivityCallRowShowsRefusalReason(t *testing.T) {
	h, repo := erpActivityFixture(t)
	repo.counts = erp.ActivityCounts{IntegrationStatus: "paused"}
	repo.calls = []erp.CallRow{{ID: 3, Route: "/orders/sales", Status: 401, ErrorCode: "ERP_AUTH_INVALID", AuthReason: "integration_status", CreatedAt: time.Now()}}
	body := erpActivityGET(t, h, "/vendor/integrations/activity?tab=calls", erpPageActor(1, true), h.ErpActivityPage)
	if !strings.Contains(body, "الربط موقوف مؤقتاً") {
		t.Fatal("refusal reason text missing from the call row")
	}
	if !strings.Contains(body, `role="alert"`) {
		t.Fatal("paused integration must show the warning on the activity screen")
	}
}

func TestActivityRoutesFollowPermissionAndAudience(t *testing.T) {
	h, _ := erpActivityFixture(t)
	vendor := func(r chi.Router) { h.registerVendorRoutes(r) }
	customer := func(r chi.Router) { h.registerCustomerRoutes(r) }
	cases := []struct {
		name     string
		register func(chi.Router)
		path     string
		want     int
	}{
		{"vendor activity", vendor, "/vendor/integrations/activity", http.StatusSeeOther},
		{"vendor live", vendor, "/vendor/integrations/activity/live", http.StatusSeeOther},
		{"vendor batch", vendor, "/vendor/integrations/activity/syncs/1", http.StatusSeeOther},
		{"pharmacy activity", customer, "/customer/integrations/activity", http.StatusSeeOther},
		{"pharmacy batch has no route", customer, "/customer/integrations/activity/syncs/1", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := chi.NewRouter()
			c.register(r)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", c.path, nil))
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

func TestActivityBatchIsNotFoundAcrossTenants(t *testing.T) {
	h, repo := erpActivityFixture(t)
	repo.batches = []erp.BatchRow{{SyncBatch: erp.SyncBatch{ID: 5, OrganizationID: 2, PublicID: "5", State: "completed"}}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/vendor/integrations/activity/syncs/5", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), erpPageActor(1, true)))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", "5")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	h.ErpBatchPage(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("another tenant's batch was rendered")
	}
}
