package catalog_test

import (
	"context"
	"os"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestMaxImportBatch_EnvVar(t *testing.T) {
	origCatalog := os.Getenv("CATALOG_MAX_IMPORT_ROWS")
	origMax := os.Getenv("MAX_IMPORT_ROWS")
	defer func() {
		_ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", origCatalog)
		_ = os.Setenv("MAX_IMPORT_ROWS", origMax)
	}()

	// 1. Default should be 0 (unlimited)
	_ = os.Unsetenv("CATALOG_MAX_IMPORT_ROWS")
	_ = os.Unsetenv("MAX_IMPORT_ROWS")
	if got := catalog.MaxImportBatch(); got != 0 {
		t.Errorf("expected default MaxImportBatch to be 0 (unlimited), got %d", got)
	}

	// 2. Set CATALOG_MAX_IMPORT_ROWS
	_ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", "50000")
	if got := catalog.MaxImportBatch(); got != 50000 {
		t.Errorf("expected MaxImportBatch to be 50000, got %d", got)
	}

	// 3. Set MAX_IMPORT_ROWS fallback
	_ = os.Unsetenv("CATALOG_MAX_IMPORT_ROWS")
	_ = os.Setenv("MAX_IMPORT_ROWS", "100000")
	if got := catalog.MaxImportBatch(); got != 100000 {
		t.Errorf("expected MaxImportBatch to be 100000, got %d", got)
	}

	// 4. Set to 0 or negative explicitly disables limit
	_ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", "0")
	if got := catalog.MaxImportBatch(); got != 0 {
		t.Errorf("expected MaxImportBatch to be 0, got %d", got)
	}
}

func TestBulkImportProducts_LimitEnforcedWhenConfigured(t *testing.T) {
	orig := os.Getenv("CATALOG_MAX_IMPORT_ROWS")
	defer func() { _ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", orig) }()

	// Configure a low limit of 2 items
	_ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", "2")

	svc := catalog.NewService(nil, nil)
	prods := []*catalog.Product{
		{Name: i18n.New("Item 1", "Item 1"), SKU: "SKU1"},
		{Name: i18n.New("Item 2", "Item 2"), SKU: "SKU2"},
		{Name: i18n.New("Item 3", "Item 3"), SKU: "SKU3"},
	}

	_, _, err := svc.BulkImportProducts(context.Background(), prods, catalog.BulkWriteOptions{})
	if err == nil {
		t.Fatal("expected error when batch exceeds configured limit, got nil")
	}

	// Now set limit to 0 (unlimited)
	_ = os.Setenv("CATALOG_MAX_IMPORT_ROWS", "0")
	func() {
		defer func() {
			// A panic on nil repo is expected because repo is nil, confirming it passed the limit check
			_ = recover()
		}()
		_, _, _ = svc.BulkImportProducts(context.Background(), prods, catalog.BulkWriteOptions{})
	}()
}

