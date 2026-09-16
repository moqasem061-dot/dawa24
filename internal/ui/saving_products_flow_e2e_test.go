package ui

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The saving-products import, driven through the real handlers.
//
// The reported failure was that the wizard "reaches 4% and hangs", having
// skipped the column-mapping step. Both halves came from one cause: the upload
// handler created the session in the `processing` state that belongs to the
// async run, so the wizard rendered the matching screen — step 2 was never
// shown — and the progress bar then polled a session frozen at Progress 5,
// which the client drift floors to 4%, with no goroutine behind it.
//
// This test walks the actual POST and GET a browser makes.

func TestSavingImportUploadLandsOnTheColumnMappingScreen(t *testing.T) {
	h := newSavingFlowHandler()
	router := savingFlowRouter(h, authctx.Actor{UserID: 200, OrganizationID: 100, Role: "customer"})

	body, contentType := savingWorkbookUpload(t)
	req := httptest.NewRequest(http.MethodPost, "/customer/saving-products/import/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload: expected 303, got %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if strings.Contains(location, "notice_type=error") {
		t.Fatalf("upload was refused: %s", location)
	}

	sessionID := location[strings.LastIndex(location, "/")+1:]
	sess, ok := globalSavingImportSessionStore.GetSession(sessionID, 100)
	if !ok {
		t.Fatalf("no session was created; redirected to %s", location)
	}

	// The two facts the wizard branches on.
	if sess.IsProcessing() {
		t.Error("nothing is matching this session yet — reporting it as processing " +
			"is what skipped the mapping screen and left the bar at 4%")
	}
	if !sess.AwaitingMapping() {
		t.Errorf("expected the session to await mapping, got status=%q phase=%q", sess.Status, sess.Phase)
	}

	// And what the screen renders.
	page := savingFlowGet(t, router, location)
	if !strings.Contains(page, "ربط الأعمدة") {
		t.Error("the column-mapping screen was not rendered after upload")
	}
	if strings.Contains(page, "جارٍ مطابقة الأصناف") {
		t.Error("the wizard jumped straight to the matching screen, skipping step 2")
	}
	// The selects the buyer has to answer, including the دوا 24 id column that
	// the mapping submit used to discard.
	for _, field := range []string{"col_name", "col_sku", "col_qty", "col_price", "col_product_id"} {
		if !strings.Contains(page, `name="`+field+`"`) {
			t.Errorf("the mapping form is missing the %s select", field)
		}
	}
	// The marker that makes an unticked AI switch mean "no".
	if !strings.Contains(page, `name="ai_choice"`) {
		t.Error("the AI switch has no presence marker, so leaving it off silently turns AI on")
	}
}

// A session parked on the mapping screen must report no progress. The bar is
// only started by the screen that follows it, and a non-zero percentage here is
// exactly what a hung import looks like.
func TestSessionAwaitingMappingReportsNoProgress(t *testing.T) {
	store := &SavingImportSessionStore{sessions: map[string]*SavingImportSession{}}
	sess := store.NewMappingSession(100, 200, "list.xlsx",
		[]string{"الصنف"}, [][]string{{"بانادول"}}, [][]string{{"بانادول"}},
		SavingDetectedCols{NameCol: 0, SKUCol: -1, QtyCol: -1, PriceCol: -1, ProductIDCol: -1})

	rec := httptest.NewRecorder()
	respondWithSavingSessionProgress(rec, sess)

	if got := rec.Body.String(); !strings.Contains(got, `"percent":0`) {
		t.Errorf("a session nobody is processing must report 0%%, got %s", got)
	}
}

// The wizard's AI switch: unticked means off, ticked means on. The drag-and-drop
// path, which posts no switch at all, keeps the platform default.
func TestWizardAISwitchIsHonouredBothWays(t *testing.T) {
	cases := []struct {
		name string
		form string
		want bool
	}{
		{"wizard, switch off", "ai_choice=1", false},
		{"wizard, switch on", "ai_choice=1&use_ai=1", true},
		{"no wizard marker at all", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if got := ParseUseAIFromWizard(req); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func newSavingFlowHandler() *UIHandler {
	return &UIHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func savingFlowRouter(h *UIHandler, actor authctx.Actor) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), actor)))
		})
	})
	r.Post("/customer/saving-products/import/upload", h.CustomerSavingProductsImportUploadSubmit)
	r.Get("/customer/saving-products/import/{id}", h.CustomerSavingProductsImportSessionPage)
	return r
}

func savingFlowGet(t *testing.T, router chi.Router, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d (%s)", path, rec.Code, rec.Header().Get("Location"))
	}
	return rec.Body.String()
}

// savingWorkbookUpload builds the multipart body a browser sends, around a real
// xlsx — the handler validates the container before it parses it, so a text
// file with an .xlsx name would be refused for the wrong reason.
func savingWorkbookUpload(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	f := excelize.NewFile()
	rows := [][]any{
		{"اسم الصنف", "كود SKU", "الكمية", "سعر الجمهور"},
		{"بانادول اكسترا 24 قرص", "PAN-24", 10, 35.50},
		{"كونجستال 20 قرص", "CONG-20", 5, 29.00},
	}
	for i, row := range rows {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue("Sheet1", cell, val)
		}
	}
	var raw bytes.Buffer
	if err := f.Write(&raw); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "list.xlsx")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(raw.Bytes()); err != nil {
		t.Fatalf("copy workbook: %v", err)
	}
	_ = w.Close()
	return &body, w.FormDataContentType()
}
