package ingest

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockCatalogAdapter struct {
	variants map[int64]*catalog.ProductVariant
	nextID   int64
}

func (m *mockCatalogAdapter) GetVariantBySKUOrBarcode(_ context.Context, orgID int64, sku, barcode string) (*catalog.ProductVariant, error) {
	for _, v := range m.variants {
		if v.OrganizationID == orgID {
			if (sku != "" && v.SKU == sku) || (barcode != "" && v.Barcode == barcode) {
				return v, nil
			}
		}
	}
	return nil, apperr.NotFound("variant")
}

func (m *mockCatalogAdapter) GetVariantByProductAndOrg(_ context.Context, orgID int64, productID int64) (*catalog.ProductVariant, error) {
	for _, v := range m.variants {
		if v.OrganizationID == orgID && v.ProductID == productID {
			return v, nil
		}
	}
	return nil, apperr.NotFound("variant")
}

func (m *mockCatalogAdapter) CreateVariant(_ context.Context, v *catalog.ProductVariant) (*catalog.ProductVariant, error) {
	m.nextID++
	v.ID = m.nextID
	m.variants[v.ID] = v
	return v, nil
}

func (m *mockCatalogAdapter) UpdateVariant(_ context.Context, id int64, v *catalog.ProductVariant) (*catalog.ProductVariant, error) {
	v.ID = id
	m.variants[id] = v
	return v, nil
}

func (m *mockCatalogAdapter) GetProduct(_ context.Context, id int64) (*catalog.Product, []*catalog.ProductVariant, error) {
	return nil, nil, nil
}

type mockInventoryAdapter struct {
	stocks       map[string]*inventory.Stock
	clearedCount int
}

func (m *mockInventoryAdapter) ClearWarehouseStocks(_ context.Context, warehouseID int64) error {
	m.clearedCount++
	m.stocks = make(map[string]*inventory.Stock)
	return nil
}

func (m *mockInventoryAdapter) UpsertStock(_ context.Context, s *inventory.Stock) error {
	key := s.WarehouseID
	m.stocks[string(rune(key))] = s
	return nil
}

func TestReconcilerImportModes(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)

	t.Run("ModeAddNewOnly skips existing variants", func(t *testing.T) {
		repo := newMockIngestRepo()
		svc := NewService(repo, nil)

		cat := &mockCatalogAdapter{
			variants: make(map[int64]*catalog.ProductVariant),
		}
		inv := &mockInventoryAdapter{
			stocks: make(map[string]*inventory.Stock),
		}

		// Existing variant for product 101
		existingVar := &catalog.ProductVariant{
			ID:             1,
			OrganizationID: 10,
			ProductID:      101,
			SKU:            "PAN-EXT-500",
		}
		cat.variants[1] = existingVar

		prodID1 := int64(101)
		prodID2 := int64(102)

		whID := int64(5)
		session := &ImportSession{
			OrganizationID: 10,
			WarehouseID:    &whID,
			ImportMode:     ModeAddNewOnly,
			ColumnMapping: map[string]string{
				FieldProductName: "اسم الصنف",
				FieldSKU:         "كود الصنف",
				FieldPrice:       "السعر",
				FieldQuantity:    "الكمية",
			},
			Status: StatusPending,
		}
		_ = repo.CreateImportSession(ctx, session)

		rows := []*ImportRow{
			{
				SessionID:        session.ID,
				OrganizationID:   10,
				RowNumber:        1,
				MatchedProductID: &prodID1,
				RawData: map[string]any{
					"اسم الصنف": "بانادول إكسترا",
					"كود الصنف": "PAN-EXT-500",
					"السعر":     "50.00",
					"الكمية":    "100",
				},
				IsApproved: true,
			},
			{
				SessionID:        session.ID,
				OrganizationID:   10,
				RowNumber:        2,
				MatchedProductID: &prodID2,
				RawData: map[string]any{
					"اسم الصنف": "كونجستال",
					"كود الصنف": "CONG-650",
					"السعر":     "30.00",
					"الكمية":    "50",
				},
				IsApproved: true,
			},
		}
		_ = repo.InsertImportRows(ctx, rows)

		outcome, err := svc.CommitSessionWithReconciliation(ctx, session.ID, cat, inv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if outcome.Skipped != 1 {
			t.Fatalf("expected 1 skipped (existing), got %d", outcome.Skipped)
		}
		if outcome.Inserted != 1 {
			t.Fatalf("expected 1 inserted (new), got %d", outcome.Inserted)
		}
	})

	t.Run("ModeClearAndAdd clears warehouse and inserts all", func(t *testing.T) {
		repo := newMockIngestRepo()
		svc := NewService(repo, nil)

		cat := &mockCatalogAdapter{
			variants: make(map[int64]*catalog.ProductVariant),
		}
		inv := &mockInventoryAdapter{
			stocks: make(map[string]*inventory.Stock),
		}

		prodID := int64(101)
		whID := int64(5)
		session := &ImportSession{
			OrganizationID: 10,
			WarehouseID:    &whID,
			ImportMode:     ModeClearAndAdd,
			ColumnMapping: map[string]string{
				FieldProductName: "اسم الصنف",
				FieldPrice:       "السعر",
				FieldQuantity:    "الكمية",
			},
			Status: StatusPending,
		}
		_ = repo.CreateImportSession(ctx, session)

		rows := []*ImportRow{
			{
				SessionID:        session.ID,
				OrganizationID:   10,
				RowNumber:        1,
				MatchedProductID: &prodID,
				RawData: map[string]any{
					"اسم الصنف": "بانادول إكسترا",
					"السعر":     "50.00",
					"الكمية":    "100",
				},
				IsApproved: true,
			},
		}
		_ = repo.InsertImportRows(ctx, rows)

		outcome, err := svc.CommitSessionWithReconciliation(ctx, session.ID, cat, inv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if inv.clearedCount != 1 {
			t.Fatalf("expected warehouse stocks to be cleared once, got %d", inv.clearedCount)
		}
		if outcome.Inserted != 1 {
			t.Fatalf("expected 1 inserted, got %d", outcome.Inserted)
		}
	})
}
