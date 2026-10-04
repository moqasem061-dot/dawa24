package compare_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestWarehouseReplacementRequiresCompleteMatching(t *testing.T) {
	for _, tc := range []struct {
		name      string
		dec       matchflow.RowDecision
		err       error
		wantError bool
	}{{"accepted", matchflow.RowDecision{State: matchflow.StateAccepted, ProductID: int64Pointer(7)}, nil, false}, {"review", matchflow.RowDecision{State: matchflow.StateNeedsReview, ProductID: int64Pointer(7)}, nil, false}, {"outage", matchflow.RowDecision{}, errors.New("catalog offline"), true}, {"unavailable", matchflow.RowDecision{State: matchflow.StateUnavailable}, nil, true}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCompareRepo()
			svc := compare.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
			svc.SetMatcher(&fakeCompareMatcher{dec: tc.dec, err: tc.err})
			file := &compare.CompareFile{UserID: 1, IsTempWarehouse: true}
			if err := repo.CreateFile(context.Background(), file); err != nil {
				t.Fatal(err)
			}
			old := &compare.CompareFileRow{FileID: file.ID, RawName: "old"}
			if err := repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{old}); err != nil {
				t.Fatal(err)
			}
			row := &compare.CompareFileRow{FileID: file.ID, RawName: "Panadol", RowNumber: 1}
			err := svc.MatchAndReplaceFileRows(context.Background(), file, []*compare.CompareFileRow{row})
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v", err)
			}
			stored, _ := repo.ListFileRows(context.Background(), file.ID, 10, 0)
			if tc.wantError {
				if len(stored) != 1 || stored[0].RawName != "old" {
					t.Fatal("failed matching erased original rows")
				}
				return
			}
			if len(row.MatchDecision) == 0 || row.MatchState != string(tc.dec.State) {
				t.Fatalf("missing decision: %+v", row)
			}
			if tc.name == "review" && row.MatchedProductID != nil {
				t.Fatal("suggestion published as accepted match")
			}
		})
	}
}
func int64Pointer(v int64) *int64 { return &v }
func TestImportPriceUsesExactDecimal(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"١٬٢٣٤٫٥٦ ج.م", "1234.56"}, {"1.234,56", "1234.56"}, {"-100", "0.00"}, {"1000000000000000", "0.00"}, {"1.005", "1.01"}} {
		if got := compare.ParseImportPrice(tc.input).String(); got != tc.want {
			t.Errorf("%q => %s want %s", tc.input, got, tc.want)
		}
	}
}
