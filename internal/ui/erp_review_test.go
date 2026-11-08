package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The supplier's review screen (S2): open items list with resolve/dismiss;
// another organization's item never visible (US1.3 applied to review rows).
// The handlers run over the real Service on a local review fake — the erp
// package's own fakes live in its test binary and are invisible here.

type reviewFakeRepo struct {
	erp.Repository
	items      map[int64]*erp.ReviewItem
	linked     int64
	remembered []string
	auditCount int
	variantID  int64
}

func (r *reviewFakeRepo) ListReview(_ context.Context, orgID int64, page erp.Page) ([]erp.ReviewItem, error) {
	var out []erp.ReviewItem
	for _, item := range r.items {
		if item.OrganizationID != orgID || item.State != "open" {
			continue
		}
		if page.After > 0 && item.ID <= page.After {
			continue
		}
		out = append(out, *item)
	}
	return out, nil
}

func (r *reviewFakeRepo) GetReview(_ context.Context, orgID, id int64) (erp.ReviewItem, error) {
	item, ok := r.items[id]
	if !ok || item.OrganizationID != orgID {
		return erp.ReviewItem{}, erp.ErrNotFound
	}
	return *item, nil
}

func (r *reviewFakeRepo) WriteResults(_ context.Context, orgID int64, res []erp.ItemResult, links []erp.LinkWrite, reviews []erp.ReviewWrite) error {
	for _, link := range links {
		r.linked = link.VariantID
	}
	return nil
}

func (r *reviewFakeRepo) ResolveReviewRow(_ context.Context, orgID, id, variantID, productID, actorUserID int64, audit erp.AuditRecord) error {
	item, ok := r.items[id]
	if !ok || item.OrganizationID != orgID || item.State != "open" {
		return erp.ErrNotFound
	}
	item.State = "resolved"
	item.ResolvedVariantID = &variantID
	item.ResolvedProductID = &productID
	r.linked = variantID
	r.auditCount++
	return nil
}

func (r *reviewFakeRepo) DismissReviewRow(_ context.Context, orgID, id, actorUserID int64, audit erp.AuditRecord) error {
	item, ok := r.items[id]
	if !ok || item.OrganizationID != orgID || item.State != "open" {
		return erp.ErrNotFound
	}
	item.State = "dismissed"
	r.auditCount++
	return nil
}

type reviewWriterFake struct{}

func (reviewWriterFake) Write(_ context.Context, _ int64, rows []erp.VariantWrite) (erp.VariantWriteResult, error) {
	result := erp.VariantWriteResult{IDs: map[int]int64{}, InsertedRefs: map[int]bool{}, Failures: map[int]string{}}
	for i, row := range rows {
		result.IDs[row.Ref] = 500 + int64(i)
		if row.VariantID == 0 {
			result.Inserted++
			result.InsertedRefs[row.Ref] = true
		} else {
			result.Updated++
		}
		_ = money.Zero
	}
	return result, nil
}

func (reviewWriterFake) SetQuotaLimits(context.Context, int64, map[int64]*int) error { return nil }

func reviewFixture(t *testing.T) (*UIHandler, *reviewFakeRepo) {
	t.Helper()
	repo := &reviewFakeRepo{
		items: map[int64]*erp.ReviewItem{
			10: {ID: 10, OrganizationID: 1, IntegrationID: 3, ExternalCode: "R-10", ReasonCode: "ERP_ITEM_MATCH_REVIEW", State: "open"},
			11: {ID: 11, OrganizationID: 1, IntegrationID: 3, ExternalCode: "R-11", ReasonCode: "ERP_ITEM_MATCH_REVIEW", State: "open"},
			12: {ID: 12, OrganizationID: 2, IntegrationID: 4, ExternalCode: "R-12", ReasonCode: "ERP_ITEM_MATCH_REVIEW", State: "open"},
		},
	}
	svc := erp.NewService(repo, erp.Ports{VariantWriter: reviewWriterFake{}},
		erp.Config{Pepper: []byte("review-pepper-32-bytes-long!"), Tag: "test",
			DefaultRate: 600, MaxBatchItems: 50000, Provider: "orgasoft"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetERP(svc)
	return handler, repo
}

// reviewTestRoutes mounts the review surface the same way the vendor registrar
// does — the review routes exist once T137 registers them.
func reviewTestRoutes(h *UIHandler) chi.Router {
	r := chi.NewRouter()
	h.registerVendorERPRoutes(r)
	return r
}

func TestErpReviewPageListsOpenItems(t *testing.T) {
	handler, _ := reviewFixture(t)
	route := reviewTestRoutes(handler)
	actor := authctx.Actor{UserID: 9, OrganizationID: 1, OrgID: 1, OrgType: "vendor", OrgStatus: "approved", Permissions: []string{"vendor.*"}}
	req := httptest.NewRequest("GET", "/vendor/integrations/review", nil)
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "R-10") || !strings.Contains(w.Body.String(), "R-11") {
		t.Error("open items missing from the page")
	}
	if strings.Contains(w.Body.String(), "R-12") {
		t.Error("another organization's review item is on the page")
	}
}

func TestErpReviewResolveAndDismiss(t *testing.T) {
	handler, repo := reviewFixture(t)
	if err := handler.erpSvc.ResolveReview(context.Background(), 1, 10, 777, 9); err != nil {
		t.Fatal(err)
	}
	if repo.items[10].State != "resolved" || repo.items[10].ResolvedVariantID == nil || *repo.items[10].ResolvedVariantID == 0 {
		t.Fatalf("item 10 = %+v, want resolved with a variant id", repo.items[10])
	}
	if repo.linked == 0 {
		t.Error("the resolution did not link the variant")
	}
	if err := handler.erpSvc.DismissReview(context.Background(), 1, 11, 9); err != nil {
		t.Fatal(err)
	}
	if repo.items[11].State != "dismissed" {
		t.Fatalf("item 11 = %+v, want dismissed", repo.items[11])
	}
	if repo.auditCount < 2 {
		t.Errorf("audit rows = %d, want resolve + dismiss", repo.auditCount)
	}
}

func TestErpReviewForeignIsNotFound(t *testing.T) {
	handler, _ := reviewFixture(t)
	if err := handler.erpSvc.ResolveReview(context.Background(), 8, 10, 777, 9); err == nil {
		t.Error("another organization resolved the item")
	}
	if err := handler.erpSvc.DismissReview(context.Background(), 8, 10, 9); err == nil {
		t.Error("another organization dismissed the item")
	}
}
