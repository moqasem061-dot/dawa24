package ui_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// waitForTempWarehouseStaging blocks until every file of the batch has left
// FileProcessing, and returns the rows they produced. It is what the admin
// screen does through /admin/user/temparte-warehouses/staging.
func waitForTempWarehouseStaging(t *testing.T, repo *mockBulkCompareRepo, ids []string) int {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		total, pending := 0, 0
		for _, raw := range ids {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				continue
			}
			f, err := repo.GetFileByID(context.Background(), id)
			if err != nil || f == nil {
				pending++
				continue
			}
			if f.Status == compare.FileProcessing {
				pending++
				continue
			}
			if f.Status == compare.FileFailed {
				t.Fatalf("file %d failed to stage: %s", id, f.ErrorMessage)
			}
			total += f.RowCount
		}
		if pending == 0 {
			return total
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("staging did not finish within the deadline")
	return 0
}
