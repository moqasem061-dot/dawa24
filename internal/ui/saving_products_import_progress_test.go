package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestSavingImportProgress_WireContract(t *testing.T) {
	rec := httptest.NewRecorder()
	sess := &pages.SavingImportSession{
		ID:            "test-sess-123",
		OrgID:         10,
		UserID:        20,
		Status:        SessionStateReady,
		Progress:      100,
		ProgressPhase: "اكتملت المعالجة",
		ProcessedRows: 50,
		TotalRows:     50,
		MatchedRows:   45,
		UnlinkedRows:  5,
		TotalQuantity: 120,
		TotalValue:    money.FromMinor(54000),
		Items: []*StagedSavingItem{
			{Index: 1, NameProduct: "بانادول", Quantity: 10, Price: money.FromMinor(5000), Included: true},
		},
	}

	respondWithSavingSessionProgress(rec, sess)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var wire map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	// import-progress.js contract:
	if pct, ok := wire["percent"].(float64); !ok || pct != 100 {
		t.Errorf("expected percent=100 (float64), got %v", wire["percent"])
	}
	if msg, ok := wire["message"].(string); !ok || msg == "" {
		t.Errorf("expected non-empty message, got %v", wire["message"])
	}
	if cur, ok := wire["current"].(float64); !ok || cur != 50 {
		t.Errorf("expected current=50, got %v", wire["current"])
	}
	if tot, ok := wire["total"].(float64); !ok || tot != 50 {
		t.Errorf("expected total=50, got %v", wire["total"])
	}
	if done, ok := wire["done"].(bool); !ok || !done {
		t.Errorf("expected done=true, got %v", wire["done"])
	}
	if ready, ok := wire["is_ready"].(bool); !ok || !ready {
		t.Errorf("expected is_ready=true, got %v", wire["is_ready"])
	}
	if state, ok := wire["state"].(string); !ok || state != "ready" {
		t.Errorf("expected state='ready', got %v", wire["state"])
	}
	if success, ok := wire["success"].(bool); !ok || !success {
		t.Errorf("expected success=true, got %v", wire["success"])
	}

	// Staged items for modal review:
	items, ok := wire["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected items array with 1 item, got %v", wire["items"])
	}
}

func TestSavingImportProgress_HandlerRoute(t *testing.T) {
	sess := globalSavingImportSessionStore.NewSessionWithID("test-sess-route-1", 100, 200, "sample.xlsx", 20)
	sess.Status = SessionStateReady
	sess.Progress = 100
	sess.ProgressPhase = "جاهز للمراجعة"
	sess.ProcessedRows = 20

	actor := authctx.Actor{
		UserID:         200,
		OrganizationID: 100,
		Role:           "customer",
	}

	h := &UIHandler{}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), actor)))
		})
	})
	r.Get("/customer/saving-products/import/session/{id}/progress", h.handleSavingProductsImportProgressJSON)

	req := httptest.NewRequest(http.MethodGet, "/customer/saving-products/import/session/"+sess.ID+"/progress", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var wire map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if wire["done"] != true {
		t.Errorf("expected done:true in handler progress response, got %v", wire["done"])
	}
	if wire["percent"] != float64(100) {
		t.Errorf("expected percent:100 in handler progress response, got %v", wire["percent"])
	}
}
