package compare_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestComparisonUsesEntitlementBeyondTenFiles(t *testing.T) {
	for _, count := range []int{1, 10, 11, 100, 101} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			repo := newMockCompareRepo()
			svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
			ent, err := svc.EntitlementFor(context.Background(), 1, 2)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, count)
			for i := range ids {
				ids[i] = int64(i + 1)
				repo.files[ids[i]] = &compare.CompareFile{ID: ids[i], UserID: 1, SupplierName: fmt.Sprint(i), Status: compare.FileReady}
			}
			_, err = svc.RunMultiSupplierComparison(context.Background(), ids)
			if (err != nil) != (count > ent.MaxActiveFiles) {
				t.Fatalf("count=%d entitlement=%d error=%v", count, ent.MaxActiveFiles, err)
			}
		})
	}
}
