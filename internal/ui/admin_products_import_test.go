package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func superAdmin() authctx.Actor {
	return authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin", Permissions: []string{"*"}}
}

// importTestTag marks every row these tests create, so a stray one is
// recognisable in the catalogue they run against.
const importTestTag = "UITEST"

// testNamespace gives one test its own SKU prefix.
//
// These run against a shared database — in practice a real catalogue of
// thousands of products — and they create, count, and delete rows by prefix. A
// single prefix for the whole file means each test counts its neighbours' rows
// and deletes rows a neighbour is still using, which is exactly the flakiness
// this replaces.
func testNamespace(t *testing.T) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, t.Name())
	return "UITEST-" + safe
}

// sessionIDPattern pulls the session id out of the redirect the upload returns.
var sessionIDPattern = regexp.MustCompile(`/admin/products/import/([0-9a-f-]{36})`)

// uploadImportFile walks the wizard from upload to staged rows and returns the
// session id.
//
// It is three steps, not one, because the wizard is: the upload only reads the
// file's shape and lands on the mapping screen, the admin confirms or corrects
// the mapping there, and only then does a background run stage anything. These
// tests drive exactly what a browser drives, so a change that breaks the flow
// breaks them.
func uploadImportFile(t *testing.T, h http.Handler, content string, settings url.Values) string {
	t.Helper()
	sessionID := analyzeImportFile(t, h, content)
	prepareImportSession(t, h, sessionID, settings)
	return sessionID
}

// analyzeImportFile performs step one and returns the new session's id. It
// asserts the mapping screen is where the admin lands: reaching the review
// screen straight from an upload is the flaw this wizard was rebuilt to fix.
func analyzeImportFile(t *testing.T, h http.Handler, content string) string {
	t.Helper()
	rec := postImportFile(t, h, "catalog.csv", content, url.Values{})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want 303; body: %s", rec.Code, truncate(rec.Body.String(), 900))
	}
	location := rec.Header().Get("Location")
	if !strings.HasSuffix(location, "/mapping") {
		t.Fatalf("upload redirected to %q, want the column-mapping screen", location)
	}
	match := sessionIDPattern.FindStringSubmatch(location)
	if match == nil {
		t.Fatalf("no session id in redirect %q", location)
	}
	return match[1]
}

// prepareImportSession performs step two's "process" action and waits for the
// background run to finish.
func prepareImportSession(t *testing.T, h http.Handler, sessionID string, settings url.Values) {
	t.Helper()
	rec := postSessionAction(t, h, sessionID, "prepare", settings)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303; body: %s", rec.Code, truncate(rec.Body.String(), 900))
	}
	waitForPreparation(t, h, sessionID)
}

// waitForPreparation blocks until the background run reports it is finished.
func waitForPreparation(t *testing.T, h http.Handler, sessionID string) {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
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
			Done    bool   `json:"done"`
			Failed  bool   `json:"failed"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &progress); err != nil {
			t.Fatalf("decode progress %q: %v", rec.Body.String(), err)
		}
		if progress.Failed {
			t.Fatalf("preparation failed: %s", progress.Message)
		}
		if progress.Done {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal("preparation did not finish within 90s")
}

// postImportFile uploads content as a multipart file with the wizard's settings.
func postImportFile(
	t *testing.T, h http.Handler, filename, content string, settings url.Values,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range settings {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatalf("write form field: %v", err)
			}
		}
	}
	part, err := writer.CreateFormFile("import_file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	ctx := authctx.WithActor(context.Background(), superAdmin())
	req, err := http.NewRequestWithContext(ctx, "POST", "/admin/products/import", &body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// postSessionAction drives one of the review screen's buttons.
func postSessionAction(
	t *testing.T, h http.Handler, sessionID, verb string, form url.Values,
) *httptest.ResponseRecorder {
	t.Helper()

	ctx := authctx.WithActor(context.Background(), superAdmin())
	req, err := http.NewRequestWithContext(ctx, "POST",
		"/admin/products/import/"+sessionID+"/"+verb, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cleanupImportedProducts(t *testing.T, db *database.DB) string {
	t.Helper()
	ns := testNamespace(t)
	// Reported, not swallowed. A cleanup that silently fails leaves rows behind
	// that make the next run of these tests measure the wrong thing — which is
	// exactly what happened while this suite was being written.
	t.Cleanup(func() {
		err := deleteWithRetry(context.Background(), db, func(txCtx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(txCtx,
				`DELETE FROM catalog.products WHERE sku LIKE $1`, ns+"%"); err != nil {
				return err
			}
			if _, err := tx.Exec(txCtx,
				`DELETE FROM catalog.brands WHERE name->>'ar' LIKE 'UITest%'`); err != nil {
				return err
			}
			// Sessions are deliberately not deleted here. Preparation runs in
			// the background, so a blanket delete by filename removes the
			// session another test is still preparing against — which shows up
			// as a foreign-key violation on its staging rows. The staging rows
			// are cleared on commit and cancel, and the reaper collects the
			// session rows themselves.
			return nil
		})
		if err != nil {
			t.Errorf("cleanup failed, leaving rows behind for the next run: %v", err)
		}
	})
	return ns
}

// deleteWithRetry runs a cleanup transaction, retrying briefly.
//
// Committing an import kicks off a background rebuild of catalog.product_index,
// which holds locks these deletes also need. Losing that race is expected and
// transient; leaving rows behind is not, because the next run of these tests
// would then measure a catalogue it did not create.
func deleteWithRetry(ctx context.Context, db *database.DB, fn func(context.Context, pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
		}
		if err = db.InTx(database.AsSystem(ctx), fn); err == nil {
			return nil
		}
	}
	return err
}

func countImportedProducts(t *testing.T, db *database.DB, ns string) int {
	t.Helper()
	var n int
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT count(*) FROM catalog.products WHERE sku LIKE $1 AND deleted_at IS NULL`,
			ns+"%").Scan(&n)
	})
	if err != nil {
		t.Fatalf("count imported products: %v", err)
	}
	return n
}

func importFixture(rows ...string) string {
	header := "اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة"
	return header + "\n" + strings.Join(rows, "\n") + "\n"
}

// defaultSettings is the wizard's own default posture: update what matches, add
// what does not, register manufacturers, infer the pharmaceutical form.
func defaultSettings() url.Values {
	return url.Values{
		"import_mode":        {"update_and_add"},
		"auto_create_brands": {"1"},
		"assign_dosage_form": {"1"},
	}
}

func threeProductFixture(ns string) string {
	return importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, ns),
		fmt.Sprintf("%s كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestTag, ns),
		fmt.Sprintf("%s اوجمنتين 1 جم,%s-3,115.00,UITest Labs", importTestTag, ns),
	)
}

// Uploading stages the file for review and writes nothing. This is the whole
// point of the confirmation step: the admin sees the outcome before it happens.
func TestAdminProductsImportStagesWithoutWriting(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(ns), defaultSettings())

	if n := countImportedProducts(t, db, ns); n != 0 {
		t.Fatalf("catalogue holds %d products after upload, want 0 — nothing may be written before confirmation", n)
	}

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"مراجعة ما سيتم تنفيذه", "بانادول اكسترا", "كيف تمت قراءة الملف"} {
		if !strings.Contains(body, want) {
			t.Errorf("the review screen does not show %q", want)
		}
	}
}

func TestAdminProductsImportCommitsAfterConfirmation(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(ns), defaultSettings())

	rec := postSessionAction(t, h, sessionID, "commit", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d, want 303; body: %s", rec.Code, truncate(rec.Body.String(), 600))
	}
	if n := countImportedProducts(t, db, ns); n != 3 {
		t.Fatalf("catalogue holds %d products after commit, want 3", n)
	}
}

func TestAdminProductsImportCancelWritesNothing(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(ns), defaultSettings())

	if rec := postSessionAction(t, h, sessionID, "cancel", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel status = %d, want 303", rec.Code)
	}
	if n := countImportedProducts(t, db, ns); n != 0 {
		t.Fatalf("catalogue holds %d products after cancelling, want 0", n)
	}

	// A cancelled session cannot then be committed behind the admin's back.
	postSessionAction(t, h, sessionID, "commit", url.Values{})
	if n := countImportedProducts(t, db, ns); n != 0 {
		t.Fatalf("a cancelled session wrote %d products, want 0", n)
	}
}
