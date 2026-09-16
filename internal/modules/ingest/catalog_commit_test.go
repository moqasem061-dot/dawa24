package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func TestCommitImportModes(t *testing.T) {
	ctx := context.Background()

	p1 := int64(101)
	p2 := int64(102)

	// Existing variant in catalog for product 101
	existingVariantKey := catalog.VariantKey{
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

	t.Run("ModeUpsert writes both existing and new variants", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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

	t.Run("ModeUpdateOnly skips variants that exist only in catalog and not in selected warehouse", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
		if invMock.writtenRows[0].Stock.ProductVariantID != 501 {
			t.Errorf("expected stock written for variant 501, got %d", invMock.writtenRows[0].Stock.ProductVariantID)
		}
	})

	t.Run("ModeUpsert adds variant existing in catalog to warehouse when absent from warehouse", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses:  []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:   inventory.StockReplace,
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
			keys:          []catalog.VariantKey{},
			nextVariantID: 2000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
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
					StockMode:           inventory.StockReplace,
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
		if invMock.writtenRows[0].Stock.Quantity != 0 || !invMock.writtenRows[0].HasQuantity {
			t.Errorf("expected stock row quantity 0 with HasQuantity true, got qty=%d hasQty=%v",
				invMock.writtenRows[0].Stock.Quantity, invMock.writtenRows[0].HasQuantity)
		}
	})
}
