package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/xuri/excelize/v2"
)

func TestXLSXUploadStoresPlainDiscountValues(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	for cell, row := range map[string][]any{
		"A1": {"sku", "product_name", "price", "discount"},
		"A2": {"1", "Panadol Extra 24 tablets", 100, 0.25},
		"A3": {"2", "Congestal 20 tablets", 100, 12.34},
	} {
		if err := book.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	style, err := book.NewStyle(&excelize.Style{NumFmt: 9})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle("Sheet1", "D2", "D3", style); err != nil {
		t.Fatal(err)
	}
	content, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	repo := newMockCompareRepo()
	svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	file, _, err := svc.UploadAndProcessCompareFile(context.Background(), 1, nil, "Supplier", "supplier.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", int64(content.Len()), "k", content.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListFileRows(context.Background(), file.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Discount != 0.25 || rows[1].Discount != 12.34 {
		t.Fatalf("discount values changed: %v, %v", rows[0].Discount, rows[1].Discount)
	}
	if rows[0].PriceAfterDiscount.String() != "99.75" || rows[1].PriceAfterDiscount.String() != "87.66" {
		t.Fatalf("incorrect net values: %s, %s", rows[0].PriceAfterDiscount, rows[1].PriceAfterDiscount)
	}
}

func TestBackgroundParseFailureCannotRestoreAnArchivedFile(t *testing.T) {
	repo := newMockCompareRepo()
	svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	file := &compare.CompareFile{UserID: 1, Status: compare.FileProcessing, OriginalFilename: "broken.xlsx"}
	if err := repo.CreateFile(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	staging := *file
	archived := *file
	archived.Status = compare.FileArchived
	if err := repo.UpdateFile(context.Background(), &archived); err != nil {
		t.Fatal(err)
	}
	if err := svc.StageUploadedRows(context.Background(), &staging, []byte("not an Excel workbook")); err == nil {
		t.Fatal("corrupt workbook accepted")
	}
	current, err := repo.GetFileByID(context.Background(), file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != compare.FileArchived {
		t.Fatalf("late parse failure changed status to %s", current.Status)
	}
}
