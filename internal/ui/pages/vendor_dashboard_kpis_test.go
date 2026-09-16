package pages_test

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorDashboardKPIs_StockHealth(t *testing.T) {
	t.Run("Zero critical stock shows safe stock and verified products count", func(t *testing.T) {
		data := pages.VendorDashboardData{
			ActiveProducts: 1484,
			LowStockCount:  0, // No products with quantity 0
		}

		var buf strings.Builder
		err := pages.VendorDashboardKPIs(data, "ar").Render(context.Background(), &buf)
		if err != nil {
			t.Fatalf("failed to render VendorDashboardKPIs: %v", err)
		}

		html := buf.String()

		// Must NOT show critical stock text when count is 0
		if strings.Contains(html, "أصناف حرجة") {
			t.Error("should not show 'أصناف حرجة' when LowStockCount is 0")
		}

		// Must show verified count and safe badge
		if !strings.Contains(html, "1484") {
			t.Error("expected verified active products count 1484")
		}
		if !strings.Contains(html, "مخزون آمن") {
			t.Error("expected 'مخزون آمن' badge when LowStockCount is 0")
		}
	})

	t.Run("Positive critical stock shows critical items count and alert link", func(t *testing.T) {
		data := pages.VendorDashboardData{
			ActiveProducts: 1484,
			LowStockCount:  5, // 5 products reached quantity 0
		}

		var buf strings.Builder
		err := pages.VendorDashboardKPIs(data, "ar").Render(context.Background(), &buf)
		if err != nil {
			t.Fatalf("failed to render VendorDashboardKPIs: %v", err)
		}

		html := buf.String()

		// Must show critical stock text with exact count
		if !strings.Contains(html, "5 أصناف حرجة") {
			t.Errorf("expected '5 أصناف حرجة', got: %s", html)
		}

		// Must link directly to out of stock filter
		if !strings.Contains(html, "/vendor/inventory/alerts?status=out_of_stock") {
			t.Error("expected link to /vendor/inventory/alerts?status=out_of_stock")
		}
	})
}
