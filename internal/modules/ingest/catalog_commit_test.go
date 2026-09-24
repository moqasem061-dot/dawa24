package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// commitModesFixture is the catalogue and rows every import-mode test starts
// from: one variant the catalogue already has and one it does not.
func commitModesFixture() (context.Context, int64, int64, VariantKey, []*RowOutcome) {
	ctx := context.Background()

	p1 := int64(101)
	p2 := int64(102)

	// Existing variant in catalog for product 101
	existingVariantKey := VariantKey{
		ID:        501,
		ProductID: p1,
		SKU:       "SKU-EXISTS",
	}

	staged := []*RowOutcome{
		{
			ID:         1,
			SourceRow:  1,
			ProductID:  &p1,
			SourceCode: "SKU-EXISTS",
			Payload: &productmatch.Row{
				Number:      1,
				SKU:         "SKU-EXISTS",
				Name:        "Product 1 Existing",
				PublicPrice: money.FromMajor(100),
				HasQuantity: true,
				Quantity:    15,
			},
		},
		{
			ID:         2,
			SourceRow:  2,
			ProductID:  &p2,
			SourceCode: "SKU-NEW",
			Payload: &productmatch.Row{
				Number:      2,
				SKU:         "SKU-NEW",
				Name:        "Product 2 New",
				PublicPrice: money.FromMajor(50),
				HasQuantity: true,
				Quantity:    30,
			},
		},
	}

	return ctx, p1, p2, existingVariantKey, staged
}

func TestCommitImportModes(t *testing.T) {
	ctx, p1, p2, existingVariantKey, staged := commitModesFixture()
	_, _, _, _, _ = ctx, p1, p2, existingVariantKey, staged

	t.Run("ModeUpsert writes both existing and new variants", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{501: true},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             1,
				PublicID:       "imp-upsert",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpsert,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-upsert")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 1 {
			t.Errorf("expected 1 updated row, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 0 {
			t.Errorf("expected 0 skipped rows, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 2 {
			t.Errorf("expected 2 variants written, got %d", len(catMock.writtenVariants))
		}
		if len(invMock.writtenRows) != 2 {
			t.Errorf("expected 2 stock rows written, got %d", len(invMock.writtenRows))
		}
	})

	t.Run("ModeAddOnly writes only new variants and skips existing", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{501: true},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             2,
				PublicID:       "imp-add-only",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeAddOnly,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-add-only")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 0 {
			t.Errorf("expected 0 updated rows, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 1 {
			t.Errorf("expected 1 variant written, got %d", len(catMock.writtenVariants))
		}
		// Verify written variant is the NEW one
		if catMock.writtenVariants[0].Variant.SKU != "SKU-NEW" {
			t.Errorf("expected written variant to be SKU-NEW, got %s", catMock.writtenVariants[0].Variant.SKU)
		}
	})

	t.Run("ModeUpdateOnly updates only existing variants and skips new", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{501: true},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             3,
				PublicID:       "imp-update-only",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpdateOnly,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-update-only")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 0 {
			t.Errorf("expected 0 inserted rows, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 1 {
			t.Errorf("expected 1 updated row, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 1 {
			t.Errorf("expected 1 variant written, got %d", len(catMock.writtenVariants))
		}
		// Verify written variant is the EXISTING one (ID = 501)
		if catMock.writtenVariants[0].Variant.ID != 501 {
			t.Errorf("expected written variant ID to be 501, got %d", catMock.writtenVariants[0].Variant.ID)
		}
	})

	t.Run("ModeReplace deactivates unmentioned variants", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{501: true},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             4,
				PublicID:       "imp-replace",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeReplace,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-replace")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 || sess.UpdatedRows != 1 {
			t.Errorf("expected 1 inserted and 1 updated, got %d ins, %d upd", sess.InsertedRows, sess.UpdatedRows)
		}
		// Verify DeactivateVariantsExcept was called with touched variants
		if len(catMock.deactivatedExcept) != 2 {
			t.Errorf("expected 2 kept variants in DeactivateVariantsExcept, got %d", len(catMock.deactivatedExcept))
		}
	})

}
