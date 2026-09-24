package ingest

import (
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The same modes against a warehouse that lacks a variant the catalogue has.
func TestCommitImportWarehouseModes(t *testing.T) {
	ctx, p1, p2, existingVariantKey, staged := commitModesFixture()
	_, _, _, _, _ = ctx, p1, p2, existingVariantKey, staged

	t.Run("ModeUpdateOnly skips variants that exist only in catalog and not in selected warehouse", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{}, // Variant 501 is NOT in this warehouse!
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             6,
				PublicID:       "imp-update-skip-catalog-only",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpdateOnly,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged[:1], // only product 1 (existing in catalog, not in warehouse)
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-update-skip-catalog-only")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if sess.UpdatedRows != 0 {
			t.Errorf("expected 0 updated rows, got %d", sess.UpdatedRows)
		}
		if sess.InsertedRows != 0 {
			t.Errorf("expected 0 inserted rows, got %d", sess.InsertedRows)
		}
		if sess.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 0 {
			t.Errorf("expected 0 variants written, got %d", len(catMock.writtenVariants))
		}
		if len(invMock.writtenRows) != 0 {
			t.Errorf("expected 0 stock rows written, got %d", len(invMock.writtenRows))
		}
	})

	t.Run("ModeAddOnly adds variant existing in catalog to warehouse when absent from warehouse", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{}, // Variant 501 is NOT in this warehouse!
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             7,
				PublicID:       "imp-add-catalog-to-wh",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeAddOnly,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged[:1], // only product 1 (existing in catalog, not in warehouse)
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-add-catalog-to-wh")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row (added to warehouse), got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 0 {
			t.Errorf("expected 0 updated rows, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 0 {
			t.Errorf("expected 0 skipped rows, got %d", sess.SkippedRows)
		}
		if len(invMock.writtenRows) != 1 {
			t.Fatalf("expected 1 stock row written to warehouse, got %d", len(invMock.writtenRows))
		}
		if invMock.writtenRows[0].VariantID != 501 {
			t.Errorf("expected stock written for variant 501, got %d", invMock.writtenRows[0].VariantID)
		}
	})

	t.Run("ModeUpsert adds variant existing in catalog to warehouse when absent from warehouse", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []Warehouse{{ID: 1}},
			inWarehouse: map[int64]bool{}, // Variant 501 is NOT in this warehouse!
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             8,
				PublicID:       "imp-upsert-catalog-to-wh",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpsert,
					StockMode:   StockReplace,
				},
			},
			stagedRows: staged[:1],
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-upsert-catalog-to-wh")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}
		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row (added to warehouse), got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 0 {
			t.Errorf("expected 0 updated rows, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 0 {
			t.Errorf("expected 0 skipped rows, got %d", sess.SkippedRows)
		}
		if len(invMock.writtenRows) != 1 {
			t.Fatalf("expected 1 stock row written to warehouse, got %d", len(invMock.writtenRows))
		}
	})

	t.Run("StockWriting handles BlankQuantityIsZero and StockMode", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []VariantKey{},
			nextVariantID: 2000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []Warehouse{{ID: 1}},
		}
		// Row with NO quantity specified
		blankQtyRow := []*RowOutcome{
			{
				ID:         10,
				SourceRow:  1,
				ProductID:  &p1,
				SourceCode: "SKU-BLANK-QTY",
				Payload: &productmatch.Row{
					Number:      1,
					SKU:         "SKU-BLANK-QTY",
					Name:        "Product Without Quantity",
					PublicPrice: money.FromMajor(120),
					HasQuantity: false,
					Quantity:    0,
				},
			},
		}

		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             5,
				PublicID:       "imp-blank-qty",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID:         1,
					Mode:                ModeUpsert,
					StockMode:           StockReplace,
					BlankQuantityIsZero: true,
				},
			},
			stagedRows: blankQtyRow,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-blank-qty")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		// Verify stock row was queued with quantity 0 and HasQuantity = true
		if len(invMock.writtenRows) != 1 {
			t.Fatalf("expected 1 stock row written, got %d", len(invMock.writtenRows))
		}
		if invMock.writtenRows[0].Quantity != 0 || !invMock.writtenRows[0].HasQuantity {
			t.Errorf("expected stock row quantity 0 with HasQuantity true, got qty=%d hasQty=%v",
				invMock.writtenRows[0].Quantity, invMock.writtenRows[0].HasQuantity)
		}
	})
}
