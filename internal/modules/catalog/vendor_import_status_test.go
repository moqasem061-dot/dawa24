package catalog

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// bulkVariantRepo records what the service handed the persistence layer.
type bulkVariantRepo struct {
	mockCatalogRepo
	written []VariantWriteRow
}

func (r *bulkVariantRepo) ListVariantKeys(_ context.Context, _ int64) ([]VariantKey, error) {
	return nil, nil
}

func (r *bulkVariantRepo) BulkWriteVariants(
	_ context.Context, _ int64, rows []VariantWriteRow,
) (VariantWriteResult, error) {
	r.written = append(r.written, rows...)
	res := VariantWriteResult{
		IDs:          map[int]int64{},
		InsertedRefs: map[int]bool{},
	}
	for _, row := range rows {
		res.IDs[row.Ref] = row.Variant.ID
	}
	return res, nil
}

// An empty status means "leave this variant's status alone", and the service
// must not turn that into "active" on its way to the database.
//
// It used to. The default was applied to every row regardless of whether it was
// an insert or an update, so a vendor's routine price import silently
// republished every variant they had taken off sale — including the ones an
// administrator had deactivated.
func TestBulkWriteVariantsDefaultsStatusForInsertsOnly(t *testing.T) {
	repo := &bulkVariantRepo{mockCatalogRepo: *newMockCatalogRepo()}
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rows := []VariantWriteRow{
		{Ref: 0, Variant: &ProductVariant{ID: 0, ProductID: 1}},  // insert
		{Ref: 1, Variant: &ProductVariant{ID: 77, ProductID: 1}}, // update
		{Ref: 2, Variant: &ProductVariant{ID: 0, ProductID: 1, Status: StatusInactive}},
	}
	if _, err := svc.BulkWriteVariants(context.Background(), 10, rows); err != nil {
		t.Fatalf("BulkWriteVariants: %v", err)
	}

	if got := repo.written[0].Variant.Status; got != StatusActive {
		t.Errorf("insert with no status = %q, want %q", got, StatusActive)
	}
	if got := repo.written[1].Variant.Status; got != "" {
		t.Errorf("update status = %q, want it left empty so the column is untouched", got)
	}
	if got := repo.written[2].Variant.Status; got != StatusInactive {
		t.Errorf("insert with an explicit status = %q, want %q", got, StatusInactive)
	}
}
