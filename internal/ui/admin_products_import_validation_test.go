package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Re-uploading a corrected price list is routine. The importer this replaced
// only ever INSERTed, so it doubled the catalogue every time.
func TestAdminProductsImportUpdatesRatherThanDuplicating(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	first := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, ns),
	)
	sessionID := uploadImportFile(t, h, first, defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("first commit status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db, ns); n != 1 {
		t.Fatalf("after first import: %d products, want 1", n)
	}

	corrected := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,60.00,UITest Pharma", importTestTag, ns),
	)
	sessionID = uploadImportFile(t, h, corrected, defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("second commit status = %d", rec.Code)
	}
	if n := countImportedProducts(t, db, ns); n != 1 {
		t.Fatalf("after re-import: %d products, want 1 (updated, not duplicated)", n)
	}

	var price string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			ns+"-1").Scan(&price)
	})
	if err != nil {
		t.Fatalf("read updated price: %v", err)
	}
	if price != "60.00" {
		t.Errorf("price after re-import = %s, want 60.00", price)
	}
}

// The add-new-only strategy must leave an existing product alone.
func TestAdminProductsImportAddNewOnlySkipsExisting(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	seed := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,55.00,UITest Pharma", importTestTag, ns),
	)
	sessionID := uploadImportFile(t, h, seed, defaultSettings())
	postSessionAction(t, h, sessionID, "commit", url.Values{})

	settings := defaultSettings()
	settings.Set("import_mode", "add_new_only")
	both := importFixture(
		fmt.Sprintf("%s بانادول اكسترا 500 مجم,%s-1,99.00,UITest Pharma", importTestTag, ns),
		fmt.Sprintf("%s كتافلام 50 مجم أقراص,%s-2,42.00,UITest Pharma", importTestTag, ns),
	)
	sessionID = uploadImportFile(t, h, both, settings)
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db, ns); n != 2 {
		t.Fatalf("catalogue holds %d products, want 2", n)
	}

	var price string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			ns+"-1").Scan(&price)
	})
	if err != nil {
		t.Fatalf("read price: %v", err)
	}
	if price != "55.00" {
		t.Errorf("price = %s, want 55.00 — add-new-only must not touch an existing product", price)
	}
}

// The archive strategy is destructive and needs its own acknowledgement, which
// is what stops it happening on a mis-click.
func TestAdminProductsImportArchiveModeRequiresConfirmation(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	settings := defaultSettings()
	settings.Set("import_mode", "clear_and_add")
	sessionID := uploadImportFile(t, h, threeProductFixture(ns), settings)

	rec := postSessionAction(t, h, sessionID, "commit", url.Values{})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("the archive strategy committed without an acknowledgement")
	}
	if !strings.Contains(rec.Body.String(), "يجب تأكيد أرشفة") {
		t.Error("the refusal does not explain that the acknowledgement is required")
	}
	if n := countImportedProducts(t, db, ns); n != 0 {
		t.Fatalf("catalogue holds %d products, want 0", n)
	}
}

// A row the admin deselects must not be written.
func TestAdminProductsImportRespectsDeselectedRows(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(ns), defaultSettings())

	var rowID int64
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT r.id FROM catalog.import_staging_rows r
			JOIN catalog.import_sessions s ON s.id = r.session_id
			WHERE s.public_id = $1 ORDER BY r.source_row LIMIT 1
		`, sessionID).Scan(&rowID)
	})
	if err != nil {
		t.Fatalf("read staged row: %v", err)
	}

	rec := postSessionAction(t, h, sessionID, fmt.Sprintf("rows/%d", rowID), url.Values{"included": {"0"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("row toggle status = %d, want 303", rec.Code)
	}
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db, ns); n != 2 {
		t.Fatalf("catalogue holds %d products, want 2 — the deselected row must be skipped", n)
	}
}

// Re-running under a corrected column mapping must change the reading without
// writing anything, which is what makes the review step worth having.
func TestAdminProductsImportReprocessesWithCorrectedColumns(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	sessionID := uploadImportFile(t, h, threeProductFixture(ns), defaultSettings())

	settings := defaultSettings()
	// Read column 3 as the public price rather than the selling price.
	settings.Set("col_public_price", "3")
	settings.Set("col_price", "0")
	if rec := postSessionAction(t, h, sessionID, "prepare", settings); rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303", rec.Code)
	}
	waitForPreparation(t, h, sessionID)
	if n := countImportedProducts(t, db, ns); n != 0 {
		t.Fatalf("re-processing wrote %d products; it must write none", n)
	}

	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	var oldPrice string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT old_price::text FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			ns+"-1").Scan(&oldPrice)
	})
	if err != nil {
		t.Fatalf("read prices: %v", err)
	}
	if oldPrice != "55.00" {
		t.Errorf("public price = %s, want 55.00 from the rebound column", oldPrice)
	}
}

func TestAdminProductsImportReportsUnreadableFile(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	// A legacy .xls is the single most common bad upload, and it deserves the
	// specific fix rather than "invalid format".
	rec := postImportFile(t, h, "old.xls",
		string(append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)),
		defaultSettings())

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ".xlsx") {
		t.Error("the screen does not tell the admin to resave the file as .xlsx")
	}
}

func TestAdminProductsImportRejectsEmptyUpload(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	rec := postImportFile(t, h, "empty.csv", "", defaultSettings())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "فارغ") {
		t.Error("the screen does not say the file was empty")
	}
}

// The header layout that used to bind the barcode column as the item code.
func TestAdminProductsImportExplainsColumnMapping(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	file := "اسم الصنف التجاري / الوصف,الباركود الدولي,كود الصنف,سعر البيع للجمهور,الشركة المصنعة\n" +
		fmt.Sprintf("%s بانادول اكسترا,6221234567890,%s-1,55.00,UITest Pharma\n", importTestTag, ns)

	sessionID := uploadImportFile(t, h, file, defaultSettings())

	rec := doGET(t, h, "/admin/products/import/"+sessionID, superAdmin())
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"كيف تمت قراءة الملف", "الباركود", "كود الصنف"} {
		if !strings.Contains(body, want) {
			t.Errorf("the review screen does not mention %q", want)
		}
	}

	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	var sku, barcode string
	err := db.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT sku, barcode FROM catalog.products WHERE sku = $1 AND deleted_at IS NULL`,
			ns+"-1").Scan(&sku, &barcode)
	})
	if err != nil {
		t.Fatalf("read imported row: %v", err)
	}
	if barcode != "6221234567890" {
		t.Errorf("barcode = %q, want the barcode column's value, not the item code", barcode)
	}
}

// A file whose header is reprinted mid-way must import every block, and the
// reprints must not arrive as products.
func TestAdminProductsImportReadsPaginatedExport(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	const header = "اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة"
	rows := []string{"قائمة أصناف - تصدير", header}
	for i := 0; i < 30; i++ {
		if i > 0 && i%10 == 0 {
			rows = append(rows, "", header)
		}
		rows = append(rows, fmt.Sprintf("%s صنف رقم %d أقراص,%s-%03d,25.00,UITest Pharma",
			importTestTag, i, ns, i))
	}

	sessionID := uploadImportFile(t, h, strings.Join(rows, "\n")+"\n", defaultSettings())
	if rec := postSessionAction(t, h, sessionID, "commit", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("commit status = %d", rec.Code)
	}

	if n := countImportedProducts(t, db, ns); n != 30 {
		t.Fatalf("catalogue holds %d products, want 30 — every block must be read and no header imported", n)
	}
}

// truncate reports why a wizard screen was rendered instead of a redirect.
//
// The screens are full HTML documents, so the first 900 bytes are all boilerplate
// and tell a failing test nothing. The banner is the part that carries the reason.
func truncate(s string, n int) string {
	// Start after the banner element's opening tag, so the tag-stripper below
	// begins outside a tag rather than half-way through one.
	if start := strings.Index(s, "import-banner-title"); start >= 0 {
		if open := strings.IndexByte(s[start:], '>'); open >= 0 {
			start += open + 1
		}
		return stripTags(s[start:min(start+n, len(s))])
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// stripTags reduces a fragment of rendered HTML to its text.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func TestAdminProductsImportAcceptsImageURLs(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)
	ns := cleanupImportedProducts(t, db)

	file := "اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة,رابط الصورة\n" +
		fmt.Sprintf("%s بانادول اكسترا,%s-img1,65.00,UITest Pharma,https://cdn.example.com/photos/panadol.jpg\n", importTestTag, ns)

	rec := postImportFile(t, h, "products_images.csv", file, defaultSettings())
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload with image URLs rejected: status = %d, want 303 redirect. Body: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/admin/products/import/") {
		t.Errorf("expected redirect to import mapping, got %q", loc)
	}
}
