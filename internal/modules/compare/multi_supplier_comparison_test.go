package compare_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestMultiSupplierComparison_WithProcessedFiles(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(701)
	orgID := int64(801)

	csvSup1 := `كود الصنف,اسم الصنف,السعر,الخصم
101,بانادول اكسترا 24 قرص,45.00,15.0
102,كونجستال 20 قرص,31.00,22.0`

	csvSup2 := `كود الصنف,اسم الصنف,السعر,الخصم
201,بانادول اكسترا 24 قرص,45.00,20.0
202,كونجستال 20 قرص,31.00,18.0`

	f1, _, err := svc.UploadAndProcessCompareFile(ctx, userID, &orgID, "مورد القاهرة", "sup1.csv", "text/csv", int64(len(csvSup1)), "k1", []byte(csvSup1))
	if err != nil {
		t.Fatalf("upload sup 1 failed: %v", err)
	}
	f2, _, err := svc.UploadAndProcessCompareFile(ctx, userID, &orgID, "مورد الجيزة", "sup2.csv", "text/csv", int64(len(csvSup2)), "k2", []byte(csvSup2))
	if err != nil {
		t.Fatalf("upload sup 2 failed: %v", err)
	}

	result, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
	if err != nil {
		t.Fatalf("RunMultiSupplierComparison failed: %v", err)
	}

	if result.Summary.TotalProducts != 2 {
		t.Errorf("expected 2 unique products compared, got %d", result.Summary.TotalProducts)
	}
	if result.Summary.TotalSuppliers != 2 {
		t.Errorf("expected 2 suppliers, got %d", result.Summary.TotalSuppliers)
	}

	// Verify best supplier calculation
	for _, row := range result.Rows {
		if strings.Contains(row.ProductName, "بانادول") {
			if row.BestSupplier != "مورد الجيزة" {
				t.Errorf("expected مورد الجيزة to be best supplier for Panadol (20%% vs 15%%), got %s", row.BestSupplier)
			}
			if row.BestDiscount != 20.0 {
				t.Errorf("expected best discount 20.0, got %f", row.BestDiscount)
			}
			if len(row.Offers) != 2 {
				t.Errorf("expected 2 offers for Panadol, got %d", len(row.Offers))
			}
		}
	}
}

func TestUploadAndProcessCompareFile_TemplateXLSX(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(999)
	orgID := int64(888)

	// Create sample Excel file identical to CompareSampleDownload
	xf := excelize.NewFile()
	sheetName := "كشف أسعار المورد"
	xf.SetSheetName("Sheet1", sheetName)
	_ = xf.SetSheetView(sheetName, 0, &excelize.ViewOptions{
		RightToLeft: func() *bool { b := true; return &b }(),
	})

	headers := []string{"كود الصنف (SKU)", "اسم الصنف الدوائي (Product Name)", "السعر الرسمي (Price)", "نسبة الخصم % (Discount)", "ملاحظات (Notes)"}
	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = xf.SetCellValue(sheetName, cell, hName)
	}

	samples := [][]any{
		{"1001", "بانادول اكسترا 24 قرص (Panadol Extra 24 Tab)", 45.00, 18.5, "متوفر كميات كبيرة"},
		{"1002", "اوجمنتين 1 جم 14 قرص (Augmentin 1g 14 Tab)", 135.00, 12.0, "خصم إضافي للطلبيات الكبيرة"},
		{"1003", "كونجستال 20 قرص (Congestal 20 Tab)", 31.00, 20.0, "عرض موسمي حصري"},
	}
	for rowIdx, row := range samples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = xf.SetCellValue(sheetName, cell, val)
		}
	}

	var buf bytes.Buffer
	if err := xf.Write(&buf); err != nil {
		t.Fatalf("failed to write xlsx: %v", err)
	}
	xlsxBytes := buf.Bytes()

	file, _, err := svc.UploadAndProcessCompareFile(ctx, userID, &orgID, "شركة الفتح للأدوية", "dawa24_supplier_template.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", int64(len(xlsxBytes)), "storage-key-123", xlsxBytes)
	if err != nil {
		t.Fatalf("UploadAndProcessCompareFile failed: %v", err)
	}

	t.Logf("File created: ID=%d, RowCount=%d, Status=%s, ErrorMessage=%s, NameCol=%v, PriceCol=%v", file.ID, file.RowCount, file.Status, file.ErrorMessage, file.MappingConfig.NameCol, file.MappingConfig.PriceCol)

	if file.RowCount != 3 {
		t.Fatalf("expected RowCount=3, got %d", file.RowCount)
	}
}

func TestSearchAcrossSuppliersAndCatalog_ThreeWayDifferentiation(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(50)
	orgID := int64(200)

	// Create 2 supplier files
	f1 := &compare.CompareFile{
		ID:               1,
		UserID:           userID,
		OrganizationID:   &orgID,
		SupplierName:     "شركة الفتح",
		OriginalFilename: "fateh.xlsx",
		Status:           compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f1)

	f2 := &compare.CompareFile{
		ID:               2,
		UserID:           userID,
		OrganizationID:   &orgID,
		SupplierName:     "ابن سينا فارما",
		OriginalFilename: "ibnsina.xlsx",
		Status:           compare.FileReady,
	}
	_ = repo.CreateFile(ctx, f2)

	// File 1 has Panadol (matched to catalog ID 45) and a custom supplement
	row1 := &compare.CompareFileRow{
		FileID:             f1.ID,
		RowNumber:          1,
		RawName:            "بانادول اكسترا 24 قرص",
		NormalizedName:     "بانادول اكسترا 24 قرص",
		SKU:                "1001",
		Price:              money.FromMajor(45),
		Discount:           18.5,
		PriceAfterDiscount: money.FromMinor(3668),
		MatchedProductID:   &[]int64{45}[0],
		MatchMethod:        compare.MatchMethodExactName,
	}
	row2 := &compare.CompareFileRow{
		FileID:             f1.ID,
		RowNumber:          2,
		RawName:            "مكمل غذائي فيتامين سي خاص بالفتح",
		NormalizedName:     "مكمل غذائي فيتامين سي خاص بالفتح",
		Price:              money.FromMajor(100),
		Discount:           10.0,
		PriceAfterDiscount: money.FromMajor(90),
		MatchMethod:        compare.MatchMethodNone,
	}
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{row1, row2})

	// Perform Search
	results, err := svc.SearchAcrossSuppliersAndCatalog(ctx, userID, &orgID, "بانادول")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches == 0 {
		t.Fatalf("expected matches for 'بانادول', got 0")
	}

	panadolItem := results.Items[0]
	if panadolItem.CatalogStatus != compare.StatusCatalogAndSuppliers {
		t.Errorf("expected CatalogStatus=%s, got %s", compare.StatusCatalogAndSuppliers, panadolItem.CatalogStatus)
	}
	if !panadolItem.InCatalog {
		t.Errorf("expected InCatalog=true")
	}
	if panadolItem.BestSupplier != "شركة الفتح" {
		t.Errorf("expected BestSupplier='شركة الفتح', got %s", panadolItem.BestSupplier)
	}
	if len(panadolItem.MissingFromSuppliers) != 1 || panadolItem.MissingFromSuppliers[0] != "ابن سينا فارما" {
		t.Errorf("expected MissingFromSuppliers=['ابن سينا فارما'], got %v", panadolItem.MissingFromSuppliers)
	}

	// Search for Congestal (in catalog candidates, but NOT in any supplier file)
	cResults, err := svc.SearchAcrossSuppliersAndCatalog(ctx, userID, &orgID, "كونجستال")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(cResults.Items) == 0 {
		t.Fatalf("expected catalog item for Congestal")
	}
	congestalItem := cResults.Items[0]
	if congestalItem.CatalogStatus != compare.StatusCatalogOnly {
		t.Errorf("expected CatalogStatus=%s for Congestal, got %s", compare.StatusCatalogOnly, congestalItem.CatalogStatus)
	}
	if len(congestalItem.MissingFromSuppliers) != 2 {
		t.Errorf("expected Congestal to be missing from both 2 active suppliers, got %v", congestalItem.MissingFromSuppliers)
	}
}

func TestEnhancedCrossSupplierProductMatching(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// File 1 from Supplier A (English and formatted)
	f1 := &compare.CompareFile{
		ID:           101,
		UserID:       1,
		SupplierName: "المورد أ",
		Status:       compare.FileReady,
		RowCount:     3,
	}
	_ = repo.CreateFile(ctx, f1)
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{
		{FileID: f1.ID, RowNumber: 1, RawName: "PANADOL EXTRA 24 TAB", Price: money.FromMajor(50), Discount: 20.0},
		{FileID: f1.ID, RowNumber: 2, RawName: "Cataflam 50 mg 20 tab", Price: money.FromMajor(60), Discount: 25.0},
		{FileID: f1.ID, RowNumber: 3, RawName: "Augmentin 1gm 14 tab", Price: money.FromMajor(120), Discount: 15.0},
	})

	// File 2 from Supplier B (Arabic and variations)
	f2 := &compare.CompareFile{
		ID:           102,
		UserID:       1,
		SupplierName: "المورد ب",
		Status:       compare.FileReady,
		RowCount:     3,
	}
	_ = repo.CreateFile(ctx, f2)
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{
		{FileID: f2.ID, RowNumber: 1, RawName: "بانادول اكسترا 24 قرص", Price: money.FromMajor(50), Discount: 22.0},
		{FileID: f2.ID, RowNumber: 2, RawName: "كتافلام 50 مجم 20 قرص", Price: money.FromMajor(60), Discount: 23.0},
		{FileID: f2.ID, RowNumber: 3, RawName: "اوجمنتين 1 جم 14 قرص", Price: money.FromMajor(120), Discount: 18.0},
	})

	// Run Multi-Supplier Comparison
	multiRes, err := svc.RunMultiSupplierComparison(ctx, []int64{f1.ID, f2.ID})
	if err != nil {
		t.Fatalf("RunMultiSupplierComparison failed: %v", err)
	}

	t.Logf("k1(Augmentin)=%q, k2(اوجمنتين)=%q", compare.GetCoreDrugMatchKeyForTest("Augmentin 1gm 14 tab"), compare.GetCoreDrugMatchKeyForTest("اوجمنتين 1 جم 14 قرص"))

	if multiRes.Summary.TotalProducts != 3 {
		t.Errorf("expected exactly 3 matched products across suppliers, got %d", multiRes.Summary.TotalProducts)
		for idx, row := range multiRes.Rows {
			t.Logf("Row %d: %s (Offers: %d)", idx, row.ProductName, len(row.Offers))
		}
	}

	for _, r := range multiRes.Rows {
		if len(r.Offers) != 2 {
			t.Errorf("product %s expected offers from both suppliers, got %d", r.ProductName, len(r.Offers))
		}
	}

	// Run Head-to-Head Comparison
	h2hRes, err := svc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
		SourceFileID: f1.ID,
		TargetFileID: f2.ID,
	})
	if err != nil {
		t.Fatalf("RunSupplierVsSupplierDetailed failed: %v", err)
	}

	if h2hRes.TotalShared != 3 {
		t.Errorf("expected 3 shared products in head-to-head, got %d", h2hRes.TotalShared)
	}
}

func TestCompare_RetentionAndPurge(t *testing.T) {
	repo := newMockCompareRepo()
	svc := compare.NewService(repo, slog.Default())
	ctx := context.Background()

	// 1. Create older file (created 40 days ago)
	fOld := &compare.CompareFile{
		ID:           201,
		UserID:       1,
		SupplierName: "مورد قديم",
		Status:       compare.FileReady,
		CreatedAt:    time.Now().AddDate(0, 0, -40),
	}
	_ = repo.CreateFile(ctx, fOld)
	_ = repo.InsertFileRows(ctx, []*compare.CompareFileRow{
		{FileID: fOld.ID, RowNumber: 1, RawName: "صنف تجريبي", SKU: ""},
	})

	// 2. Create fresh file (created 2 days ago)
	fFresh := &compare.CompareFile{
		ID:           202,
		UserID:       1,
		SupplierName: "مورد جديد",
		Status:       compare.FileReady,
		CreatedAt:    time.Now().AddDate(0, 0, -2),
	}
	_ = repo.CreateFile(ctx, fFresh)

	// 3. Run Purge with 30 days retention
	purged, err := svc.PurgeExpiredFiles(ctx, 30)
	if err != nil {
		t.Fatalf("PurgeExpiredFiles failed: %v", err)
	}
	if purged != 1 {
		t.Errorf("expected 1 file purged, got %d", purged)
	}

	// 4. Verify old file is gone and fresh file remains
	files, _ := svc.ListFiles(ctx, 1, nil, nil)
	if len(files) != 1 || files[0].ID != 202 {
		t.Errorf("expected only fresh file (202) to remain, got %d files", len(files))
	}
}
