package ui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The mapping step: what it shows, and that nothing is staged before it.
//
// The wizard used to upload and process in one request, so an admin met the
// result of a mapping they had never seen — and when the mapping was wrong,
// met a review screen reporting nothing found with nothing to explain it. These
// lock down the step that fixes it.
func TestAdminProductsImportShowsColumnsBeforeProcessing(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := analyzeImportFile(t, h, threeProductFixture(ns))

	rec := doGET(t, h, "/admin/products/import/"+sessionID+"/mapping", superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("mapping status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// The three things the screen exists to answer: how the file was read,
	// which column feeds which field, and what the products come out as.
	for _, want := range []string{
		"ربط الأعمدة بالحقول", // the mapping table
		"معاينة أول",          // the preview table
		"اسم الصنف",           // a field label with a chooser
		"بانادول اكسترا",      // a product read out of the file
		"بدء المعالجة",        // the button that starts a run
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the mapping screen does not show %q", want)
		}
	}

	// Nothing is staged until the admin asks for it.
	if n := countStagedRows(t, db, sessionID); n != 0 {
		t.Errorf("%d rows staged before the admin pressed process; want 0", n)
	}
}

// Landing on the review screen before a run has been asked for must not read as
// "the import found nothing". It is the mapping step that is unfinished, and
// that is where the admin belongs.
func TestAdminProductsImportReviewRedirectsBeforeProcessing(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := analyzeImportFile(t, h, threeProductFixture(ns))

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("review status = %d, want 303 to the mapping screen", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.HasSuffix(location, "/mapping") {
		t.Errorf("redirected to %q, want the mapping screen", location)
	}
}

// The analysis counts must survive the round trip to the database. Not writing
// them is why a session awaiting its mapping reported nought rows and nought
// columns for a file of thousands.
func TestAdminProductsImportRecordsFileShapeAtUpload(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := analyzeImportFile(t, h, threeProductFixture(ns))

	var totalRows int
	var structure string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT total_rows, structure::text FROM catalog.import_sessions WHERE public_id = $1`,
			sessionID).Scan(&totalRows, &structure)
	})
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if totalRows != 4 {
		t.Errorf("total_rows = %d, want 4 (a header and three products)", totalRows)
	}
	if !strings.Contains(structure, "name_ar") {
		t.Errorf("the stored structure does not name the bound fields: %s", truncate(structure, 300))
	}
}

// A mapping that reads nothing must say so and stage nothing, rather than
// leaving the admin on a review screen full of zeros.
func TestAdminProductsImportRefusesAMappingThatReadsNothing(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := analyzeImportFile(t, h, threeProductFixture(ns))

	// A row range past the end of the file: the mistake an admin makes when
	// they mean to skip a title block and overshoot.
	settings := defaultSettings()
	settings.Set("first_data_row", "100")

	rec := postSessionAction(t, h, sessionID, "preview", settings)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "لن يُقرأ أي صنف") {
		t.Error("the preview does not warn that the mapping reads nothing")
	}
	if n := countStagedRows(t, db, sessionID); n != 0 {
		t.Errorf("%d rows staged by a preview; a preview must stage nothing", n)
	}
}

// countStagedRows is how many rows a session currently holds for review.
func countStagedRows(t *testing.T, db *database.DB, sessionID string) int {
	t.Helper()
	var n int
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT count(*) FROM catalog.import_staging_rows r
			JOIN catalog.import_sessions s ON s.id = r.session_id
			WHERE s.public_id = $1`, sessionID).Scan(&n)
	})
	if err != nil {
		t.Fatalf("count staged rows: %v", err)
	}
	return n
}

// A run that stops must leave a way forward. The wizard only drew its cards for
// a reviewable session, so a failure left the admin on a page with nothing on
// it and no route but re-uploading the file.
func TestAdminProductsImportRecoversFromAFailedRun(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := analyzeImportFile(t, h, threeProductFixture(ns))

	// A row range past the end of the file: the run finds nothing and stops.
	broken := defaultSettings()
	broken.Set("first_data_row", "500")
	if rec := postSessionAction(t, h, sessionID, "prepare", broken); rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303", rec.Code)
	}
	waitForFailure(t, h, sessionID)

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "تصحيح ربط الأعمدة") {
		t.Error("a failed run offers no route back to the mapping screen")
	}
	if !strings.Contains(body, "مراجعة الأعمدة") {
		t.Error("the failure does not name the mapping step as the place to fix it")
	}

	// And the correction must actually work, without re-uploading.
	prepareImportSession(t, h, sessionID, defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit after correction = %d, want 303; body: %s",
			rec.Code, truncate(rec.Body.String(), 600))
	}
	if n := countImportedProducts(t, db, ns); n != 3 {
		t.Errorf("catalogue holds %d products, want 3 after correcting and re-running", n)
	}
}

// waitForFailure blocks until the background run reports it stopped.
func waitForFailure(t *testing.T, h http.Handler, sessionID string) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ctx := authctx.WithActor(context.Background(), superAdmin())
		req, err := http.NewRequestWithContext(ctx, "GET",
			"/admin/products/import/"+sessionID+"/progress", nil)
		if err != nil {
			t.Fatalf("create progress request: %v", err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var progress struct {
			Done   bool `json:"done"`
			Failed bool `json:"failed"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &progress); err != nil {
			t.Fatalf("decode progress %q: %v", rec.Body.String(), err)
		}
		if progress.Failed {
			return
		}
		if progress.Done {
			t.Fatal("the run reported success; it was given a mapping that reads nothing")
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal("the run neither finished nor failed within 60s")
}
