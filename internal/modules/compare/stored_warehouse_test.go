package compare

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestStoredWarehouseUsesExactFileInConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UPLOAD_DIR", root)
	dir := filepath.Join(root, "temp_warehouses")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one.csv"), []byte("owned workbook"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	file := &CompareFile{IsTempWarehouse: true, StorageKey: "/uploads/temp_warehouses/one.csv", OriginalFilename: "supplier.csv"}
	reader := svc.openStoredUpload(context.Background(), file)
	if reader == nil {
		t.Fatal("configured warehouse path was not found")
	}
	body, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(body) != "owned workbook" {
		t.Fatalf("%s %v", body, err)
	}
	file.StorageKey = "/uploads/temp_warehouses/missing.csv"
	if reader := svc.openStoredUpload(context.Background(), file); reader != nil {
		reader.Close()
		t.Fatal("missing file was substituted with another warehouse")
	}
}
