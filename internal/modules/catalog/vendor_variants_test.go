package catalog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockVendorVariantsRepo struct {
	mockCatalogRepo
	variants []*ProductVariant
	stocks   map[int64]int
}

func newMockVendorVariantsRepo() *mockVendorVariantsRepo {
	base := newMockCatalogRepo()
	return &mockVendorVariantsRepo{
		mockCatalogRepo: *base,
		stocks:          make(map[int64]int),
	}
}

func (m *mockVendorVariantsRepo) ListVendorVariants(
	_ context.Context, orgID int64, params VendorVariantQuery,
) ([]*ProductVariant, int, error) {
	var filtered []*ProductVariant
	for _, v := range m.variants {
		if v.OrganizationID != orgID {
			continue
		}
		if params.Status != "" && string(v.Status) != params.Status {
			continue
		}
		stock := m.stocks[v.ID]
		v.StockQty = stock

		switch params.Stock {
		case StockFilterIn:
			if stock <= 0 {
				continue
			}
		case StockFilterOut:
			if stock > 0 {
				continue
			}
		case StockFilterLow:
			if stock <= 0 || stock > 5 {
				continue
			}
		}

		if params.Expiring {
			if v.ExpiryDate == nil || v.ExpiryDate.After(time.Now().AddDate(0, 0, 90)) {
				continue
			}
		}

		filtered = append(filtered, v)
	}

	total := len(filtered)
	limit, offset := params.Page()
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (m *mockVendorVariantsRepo) VendorVariantStats(
	_ context.Context, orgID int64,
) (VendorVariantStats, error) {
	var stats VendorVariantStats
	for _, v := range m.variants {
		if v.OrganizationID != orgID {
			continue
		}
		stats.Total++
		if v.Status == StatusActive {
			stats.Active++
		}
		stock := m.stocks[v.ID]
		if stock > 0 {
			stats.InStock++
			if stock <= 5 {
				stats.LowStock++
			}
		} else {
			stats.OutOfStock++
		}
		if v.ExpiryDate != nil && !v.ExpiryDate.After(time.Now().AddDate(0, 0, 90)) {
			stats.Expiring++
		}
	}
	return stats, nil
}

func (m *mockVendorVariantsRepo) ListProductsByIDs(
	_ context.Context, ids []int64,
) (map[int64]*Product, error) {
	out := make(map[int64]*Product, len(ids))
	for _, id := range ids {
		if p, ok := m.products[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func TestVendorVariantQueryPageClamping(t *testing.T) {
	tests := []struct {
		name       string
		query      VendorVariantQuery
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "default values",
			query:      VendorVariantQuery{},
			wantLimit:  DefaultPageSize,
			wantOffset: 0,
		},
		{
			name: "valid page and limit",
			query: VendorVariantQuery{
				PageNumber: 3,
				PerPage:    100,
			},
			wantLimit:  100,
			wantOffset: 200,
		},
		{
			name: "invalid limit falls back to default 25",
			query: VendorVariantQuery{
				PageNumber: 2,
				PerPage:    99999,
			},
			wantLimit:  DefaultPageSize,
			wantOffset: 25,
		},
		{
			name: "negative or zero page becomes 1",
			query: VendorVariantQuery{
				PageNumber: -5,
				PerPage:    25,
			},
			wantLimit:  25,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset := tt.query.Page()
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("Page() = (%d, %d), want (%d, %d)", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestVendorVariantsServicePaginationAndStats(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockVendorVariantsRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Create master products
	p1, _ := svc.CreateProduct(ctx, &Product{
		ID:    101,
		Name:  i18n.New("بانادول إكسترا", "Panadol Extra"),
		Price: money.MustParse("35.00"),
	})
	p2, _ := svc.CreateProduct(ctx, &Product{
		ID:    102,
		Name:  i18n.New("كونجستال أقراص", "Congestal Tablets"),
		Price: money.MustParse("28.00"),
	})
	repo.products[101] = p1
	repo.products[102] = p2

	// Populate 70 variants for org 10
	now := time.Now()
	for i := 1; i <= 70; i++ {
		pID := int64(101)
		if i%2 == 0 {
			pID = 102
		}
		exp := now.AddDate(1, 0, 0)
		stock := 50
		if i <= 5 {
			stock = 0 // Out of stock
		} else if i <= 15 {
			stock = 3 // Low stock
		}
		if i <= 10 {
			exp = now.AddDate(0, 1, 0) // Expiring in 30 days
		}

		v := &ProductVariant{
			ID:             int64(i),
			OrganizationID: 10,
			ProductID:      pID,
			Name:           i18n.New(fmt.Sprintf("عرض توريد %d", i), fmt.Sprintf("Supply Offer %d", i)),
			Price:          money.MustParse("30.00"),
			Status:         StatusActive,
			ExpiryDate:     &exp,
		}
		repo.variants = append(repo.variants, v)
		repo.stocks[int64(i)] = stock
	}

	// 1. Check stats across the whole 70 variants
	stats, err := svc.VendorVariantStats(ctx, 10)
	if err != nil {
		t.Fatalf("VendorVariantStats failed: %v", err)
	}
	if stats.Total != 70 {
		t.Errorf("got total %d, want 70", stats.Total)
	}
	if stats.OutOfStock != 5 {
		t.Errorf("got out of stock %d, want 5", stats.OutOfStock)
	}
	if stats.LowStock != 10 {
		t.Errorf("got low stock %d, want 10", stats.LowStock)
	}
	if stats.InStock != 65 {
		t.Errorf("got in stock %d, want 65", stats.InStock)
	}
	if stats.Expiring != 10 {
		t.Errorf("got expiring %d, want 10", stats.Expiring)
	}

	// 2. Page 1 with PerPage 25
	p1List, total, err := svc.ListVendorVariants(ctx, 10, VendorVariantQuery{
		PageNumber: 1,
		PerPage:    25,
	})
	if err != nil {
		t.Fatalf("ListVendorVariants page 1 failed: %v", err)
	}
	if total != 70 {
		t.Errorf("got total %d, want 70", total)
	}
	if len(p1List) != 25 {
		t.Errorf("got page 1 count %d, want 25", len(p1List))
	}

	// 3. Page 3 with PerPage 25 (last page: 20 items)
	p3List, total, err := svc.ListVendorVariants(ctx, 10, VendorVariantQuery{
		PageNumber: 3,
		PerPage:    25,
	})
	if err != nil {
		t.Fatalf("ListVendorVariants page 3 failed: %v", err)
	}
	if len(p3List) != 20 {
		t.Errorf("got page 3 count %d, want 20", len(p3List))
	}

	// 4. ProductsByIDs batch lookup
	prods, err := svc.ProductsByIDs(ctx, []int64{101, 102})
	if err != nil {
		t.Fatalf("ProductsByIDs failed: %v", err)
	}
	if len(prods) != 2 || prods[101] == nil || prods[102] == nil {
		t.Errorf("ProductsByIDs did not return both products: %v", prods)
	}
}
