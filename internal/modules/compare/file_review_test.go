package compare_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestReviewColumnsWithoutGateway(t *testing.T) {
	for _, tc := range []struct {
		name, contents      string
		archived, wantError bool
	}{
		{"detected", "code,name,price,discount\nP1,Panadol 500 mg,123.45,10\n", false, false},
		{"archived", "code,name,price,discount\nP1,Panadol 500 mg,123.45,10\n", true, false},
		{"ambiguous headers", "a,b,c\nx,y,z\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCompareRepo()
			svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
			path := filepath.Join(t.TempDir(), "warehouse.csv")
			if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			file := &compare.CompareFile{UserID: 1, IsTempWarehouse: true, OriginalFilename: "warehouse.csv", StorageKey: path, Status: compare.FileReady}
			if tc.archived {
				now := time.Now()
				file.Status = compare.FileArchived
				file.ArchivedAt = &now
				file.ArchiveReason = "retain"
			}
			if err := repo.CreateFile(context.Background(), file); err != nil {
				t.Fatal(err)
			}
			if err := repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{{FileID: file.ID, RawName: "previous"}}); err != nil {
				t.Fatal(err)
			}
			err := svc.ReviewFileColumns(context.Background(), file.ID)
			if (err != nil) != tc.wantError {
				t.Fatalf("review error=%v", err)
			}
			rows, err := repo.ListFileRows(context.Background(), file.ID, 10, 0)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%v error=%v", rows, err)
			}
			if tc.wantError {
				if rows[0].RawName != "previous" {
					t.Fatal("failed review erased previous products")
				}
				return
			}
			if rows[0].Price.String() != "123.45" || len(rows[0].MatchDecision) == 0 {
				t.Fatalf("rows not parsed and matched: %+v", rows[0])
			}
			if tc.archived && (file.Status != compare.FileArchived || file.ArchivedAt == nil || file.ArchiveReason != "retain") {
				t.Fatal("review restored archived warehouse")
			}
		})
	}
}
