package ui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestCompare_PharmacyAccessible(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	pharmacyActor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "customer", Permissions: []string{"pharmacy.*"}}
	vendorActor := authctx.Actor{UserID: 101, OrganizationID: 201, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// 1. Compare Tool Page should NOT be accessible for pharmacy (redirects to dashboard)
	reqTool := httptest.NewRequest("GET", "/compare/tool", nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), pharmacyActor))
	recTool := httptest.NewRecorder()
	h.CompareToolPage(recTool, reqTool)
	if recTool.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther for pharmacy on compare tool, got %d", recTool.Code)
	}

	// 2. Market Discounts Page should NOT be accessible for pharmacy (redirects to dashboard)
	reqMarket := httptest.NewRequest("GET", "/market-discounts", nil)
	reqMarket = reqMarket.WithContext(authctx.WithActor(reqMarket.Context(), pharmacyActor))
	recMarket := httptest.NewRecorder()
	h.MarketDiscountsPage(recMarket, reqMarket)
	if recMarket.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther for pharmacy on market discounts, got %d", recMarket.Code)
	}

	// 3. Compare Tool and Market Discounts should be accessible for vendor
	reqVendorTool := httptest.NewRequest("GET", "/compare/tool", nil)
	reqVendorTool = reqVendorTool.WithContext(authctx.WithActor(reqVendorTool.Context(), vendorActor))
	recVendorTool := httptest.NewRecorder()
	h.CompareToolPage(recVendorTool, reqVendorTool)
	if recVendorTool.Code != http.StatusOK {
		t.Errorf("expected 200 OK for vendor on compare tool, got %d", recVendorTool.Code)
	}
}

func TestCompareWizard_MultiFileQueueProgression(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Create 4 files in repository
	for _, id := range []int64{101, 102, 103, 104} {
		f := &compare.CompareFile{
			ID:               id,
			UserID:           100,
			OrganizationID:   &[]int64{200}[0],
			SupplierName:     fmt.Sprintf("مورد تجريبي %d", id),
			OriginalFilename: fmt.Sprintf("file_%d.xlsx", id),
			Status:           compare.FileReady,
		}
		repo.files[id] = f
		repo.fileRows[id] = []*compare.CompareFileRow{
			{FileID: id, RawName: "صنف تجريبي", Price: money.FromMajor(100), Discount: 10},
		}
	}

	actor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// STEP 1: Open modal for file 101 with 4-file queue
	req1 := httptest.NewRequest("GET", "/compare/files/101/mapping-modal?setup=1&queue=101,102,103,104&step=1&total=4", nil)
	req1 = req1.WithContext(authctx.WithActor(req1.Context(), actor))
	rCtx1 := chi.NewRouteContext()
	rCtx1.URLParams.Add("id", "101")
	req1 = req1.WithContext(context.WithValue(req1.Context(), chi.RouteCtxKey, rCtx1))

	rec1 := httptest.NewRecorder()
	h.CompareFileMappingModal(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("step 1 modal failed: %d", rec1.Code)
	}
	body1 := rec1.Body.String()
	if !strings.Contains(body1, `name="next_file_id" value="102"`) {
		t.Errorf("step 1 expected next_file_id 102, got body:\n%s", body1)
	}

	// Submit file 101 via JSON
	form1 := url.Values{}
	form1.Set("supplier_name", "مورد تجريبي 101")
	form1.Set("name_col", "0")
	form1.Set("price_col", "1")
	form1.Set("setup_queue", "102,103,104")
	form1.Set("step", "1")
	form1.Set("total", "4")
	reqPost1 := httptest.NewRequest("POST", "/compare/files/101/mapping", strings.NewReader(form1.Encode()))
	reqPost1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost1.Header.Set("Accept", "application/json")
	reqPost1.Header.Set("HX-Request", "true")
	reqPost1 = reqPost1.WithContext(authctx.WithActor(reqPost1.Context(), actor))
	reqPost1 = reqPost1.WithContext(context.WithValue(reqPost1.Context(), chi.RouteCtxKey, rCtx1))

	recPost1 := httptest.NewRecorder()
	h.CompareFileMappingSubmit(recPost1, reqPost1)
	if recPost1.Code != http.StatusOK {
		t.Fatalf("step 1 submit failed: %d", recPost1.Code)
	}
	var res1 map[string]any
	_ = json.Unmarshal(recPost1.Body.Bytes(), &res1)
	if nextID, ok := res1["next_file_id"].(float64); !ok || int64(nextID) != 102 {
		t.Errorf("step 1 expected next_file_id 102 in json, got: %v", res1["next_file_id"])
	}

	// STEP 2: Open modal for file 102 with queue "103,104"
	// THIS PREVIOUSLY FAILED AND SKIPPED ALL REMAINING FILES!
	req2 := httptest.NewRequest("GET", "/compare/files/102/mapping-modal?setup=1&queue=103,104&step=2&total=4", nil)
	req2 = req2.WithContext(authctx.WithActor(req2.Context(), actor))
	rCtx2 := chi.NewRouteContext()
	rCtx2.URLParams.Add("id", "102")
	req2 = req2.WithContext(context.WithValue(req2.Context(), chi.RouteCtxKey, rCtx2))

	rec2 := httptest.NewRecorder()
	h.CompareFileMappingModal(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("step 2 modal failed: %d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if !strings.Contains(body2, `name="next_file_id" value="103"`) {
		t.Errorf("step 2 expected next_file_id 103, got body:\n%s", body2)
	}

	// Submit file 102 via JSON
	form2 := url.Values{}
	form2.Set("supplier_name", "مورد تجريبي 102")
	form2.Set("name_col", "0")
	form2.Set("price_col", "1")
	form2.Set("setup_queue", "103,104")
	form2.Set("step", "2")
	form2.Set("total", "4")
	reqPost2 := httptest.NewRequest("POST", "/compare/files/102/mapping", strings.NewReader(form2.Encode()))
	reqPost2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost2.Header.Set("Accept", "application/json")
	reqPost2 = reqPost2.WithContext(authctx.WithActor(reqPost2.Context(), actor))
	reqPost2 = reqPost2.WithContext(context.WithValue(reqPost2.Context(), chi.RouteCtxKey, rCtx2))

	recPost2 := httptest.NewRecorder()
	h.CompareFileMappingSubmit(recPost2, reqPost2)
	if recPost2.Code != http.StatusOK {
		t.Fatalf("step 2 submit failed: %d", recPost2.Code)
	}
	var res2 map[string]any
	_ = json.Unmarshal(recPost2.Body.Bytes(), &res2)
	if nextID, ok := res2["next_file_id"].(float64); !ok || int64(nextID) != 103 {
		t.Errorf("step 2 expected next_file_id 103 in json, got: %v", res2["next_file_id"])
	}

	// STEP 3: Open modal for file 103 with queue "104"
	req3 := httptest.NewRequest("GET", "/compare/files/103/mapping-modal?setup=1&queue=104&step=3&total=4", nil)
	req3 = req3.WithContext(authctx.WithActor(req3.Context(), actor))
	rCtx3 := chi.NewRouteContext()
	rCtx3.URLParams.Add("id", "103")
	req3 = req3.WithContext(context.WithValue(req3.Context(), chi.RouteCtxKey, rCtx3))

	rec3 := httptest.NewRecorder()
	h.CompareFileMappingModal(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("step 3 modal failed: %d", rec3.Code)
	}
	body3 := rec3.Body.String()
	if !strings.Contains(body3, `name="next_file_id" value="104"`) {
		t.Errorf("step 3 expected next_file_id 104, got body:\n%s", body3)
	}

	// STEP 4: Open modal for file 104 with empty queue (last file)
	req4 := httptest.NewRequest("GET", "/compare/files/104/mapping-modal?setup=1&queue=&step=4&total=4", nil)
	req4 = req4.WithContext(authctx.WithActor(req4.Context(), actor))
	rCtx4 := chi.NewRouteContext()
	rCtx4.URLParams.Add("id", "104")
	req4 = req4.WithContext(context.WithValue(req4.Context(), chi.RouteCtxKey, rCtx4))

	rec4 := httptest.NewRecorder()
	h.CompareFileMappingModal(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("step 4 modal failed: %d", rec4.Code)
	}
	body4 := rec4.Body.String()
	if !strings.Contains(body4, `name="next_file_id" value="0"`) {
		t.Errorf("step 4 expected next_file_id 0 (final file), got body:\n%s", body4)
	}
	if !strings.Contains(body4, "حفظ وبدء المطابقة") {
		t.Errorf("step 4 expected final submit button text, got body:\n%s", body4)
	}
}

// waitForStaging blocks until the detached parse has recorded its outcome.
//
// Polling rather than a channel, because the thing under test is a goroutine
// the handler deliberately does not hand back — the screen learns the outcome
// by asking, and so does this.
func waitForStaging(t *testing.T, repo *mockCompareRepoE2E, fileID int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if f := repo.fileOf(fileID); f != nil && f.Status != compare.FileProcessing {
			if f.Status == compare.FileFailed {
				t.Fatalf("staging failed: %s", f.ErrorMessage)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %d never left %q", fileID, compare.FileProcessing)
}
