package pages_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorEarningsOrderPage_Pagination(t *testing.T) {
	shipments := []*commerce.VendorShipmentProfit{
		{
			ShipmentID:      101,
			ShipmentNumber:  "SHP-001",
			OrderID:         201,
			OrderNumber:     "ORD-201",
			CustomerOrgName: "صيدلية التحرير",
			DeliveredAt:     time.Now(),
			GrossSales:      money.FromMajor(1000),
			Discounts:       money.FromMajor(100),
			NetSales:        money.FromMajor(900),
			COGS:            money.FromMajor(700),
			PlatformFee:     money.FromMajor(0),
			NetProfit:       money.FromMajor(200),
			ProfitMargin:    22.2,
			PaymentStatus:   "paid",
		},
	}

	products := []*commerce.VendorProductProfit{
		{
			ProductID:              1,
			Name:                   "بانادول إكسترا",
			SKU:                    "PAN-01",
			QuantitySold:           50,
			SellingPrice:           money.FromMajor(50),
			DiscountedCost:         money.FromMajor(40),
			TotalRevenue:           money.FromMajor(2500),
			TotalCost:              money.FromMajor(2000),
			NetProfit:              money.FromMajor(500),
			ProfitMargin:           20.0,
			CostDiscountPercentage: 10,
		},
	}

	summary := &commerce.VendorFinancialSummary{
		Period:               "month",
		GrossSales:           money.FromMajor(1000),
		TotalDiscounts:       money.FromMajor(100),
		NetSales:             money.FromMajor(900),
		COGS:                 money.FromMajor(700),
		NetProfit:            money.FromMajor(200),
		ProfitMargin:         22.2,
		DeliveredOrdersCount: 1,
		Shipments:            shipments,
		TopProducts:          products,
	}

	qVals := url.Values{}
	qVals.Set("period", "month")

	ordersPagination := components.PaginationProps{
		CurrentPage: 1,
		PageSize:    15,
		TotalCount:  45, // 3 pages
		BaseURL:     "/vendor/earnings/order",
		QueryValues: qVals,
		PageParam:   "page_orders",
		SizeParam:   "limit_orders",
	}

	productsPagination := components.PaginationProps{
		CurrentPage: 2,
		PageSize:    10,
		TotalCount:  30, // 3 pages
		BaseURL:     "/vendor/earnings/order",
		QueryValues: qVals,
		PageParam:   "page_products",
		SizeParam:   "limit_products",
	}

	data := pages.VendorEarningsOrderPageData{
		Summary:            summary,
		PagedShipments:     shipments,
		OrdersPagination:   ordersPagination,
		PagedProducts:      products,
		ProductsPagination: productsPagination,
		Lang:               "ar",
		Dir:                "rtl",
	}

	var buf strings.Builder
	err := pages.VendorEarningsOrderPage(data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render VendorEarningsOrderPage: %v", err)
	}

	html := buf.String()

	// 1. Verify shipments table renders row and pagination
	if !strings.Contains(html, "SHP-001") {
		t.Error("expected SHP-001 in orders table")
	}
	if !strings.Contains(html, "page_orders=") {
		t.Error("expected page_orders parameter in shipments table pagination links")
	}

	// 2. Verify products table renders row and pagination
	if !strings.Contains(html, "بانادول إكسترا") {
		t.Error("expected product name in products table")
	}
	if !strings.Contains(html, "page_products=") {
		t.Error("expected page_products parameter in products table pagination links")
	}

	// 3. Verify that cols-4 is used for KPI cards and cols-5 is completely gone
	if !strings.Contains(html, "dashboard-stat-grid cols-4") {
		t.Error("expected dashboard-stat-grid cols-4 for executive KPI cards")
	}
	if strings.Contains(html, "cols-5") {
		t.Error("cols-5 should have been replaced with cols-4")
	}

	// 4. Verify that Profit On Cost / ROI is completely removed from all tables and KPI cards
	if strings.Contains(html, "th_profit_on_cost") || strings.Contains(html, "العائد على التكلفة") {
		t.Error("العائد على التكلفة / ProfitOnCost must be completely removed from the page")
	}

	// 5. Verify quantity formatting has no Go format error (e.g. %!d(string=...))
	if strings.Contains(html, "%!d") || strings.Contains(html, "%!s") {
		t.Errorf("found Go format string mismatch in rendered html")
	}
	if !strings.Contains(html, "50 عبوة") {
		t.Errorf("expected '50 عبوة' in rendered products table")
	}
}

func TestVendorEarnings_AccountingFormulasAndZeroCostHandling(t *testing.T) {
	// 1. Commerce OrderLine with Cost: Public 100, Selling 90 (discount 10), CostPrice 70, CostDiscount 25% on Public
	cost := money.FromMajor(70)
	lineWithCost := &commerce.OrderLine{
		Quantity:               1,
		UnitPrice:              money.FromMajor(90),
		TotalPrice:             money.FromMajor(90),
		ListPrice:              money.FromMajor(100),
		CostPrice:              &cost,
		CostDiscountPercentage: 25.0,
	}

	// Effective purchase cost on public price: 100 - (100 * 0.25) = 75
	effCost := lineWithCost.EffectivePurchaseCost()
	if effCost.Minor() != 7500 {
		t.Fatalf("expected effective cost 7500, got %d", effCost.Minor())
	}

	// Total cost must strictly be purchase cost (75), NEVER adding selling discount (10)
	totCost := lineWithCost.TotalCost()
	if totCost.Minor() != 7500 {
		t.Fatalf("expected total cost 7500 without selling discount, got %d", totCost.Minor())
	}

	// Net profit: 90 - 75 = 15
	netProfit := lineWithCost.TotalNetProfit()
	if netProfit.Minor() != 1500 {
		t.Fatalf("expected net profit 1500, got %d", netProfit.Minor())
	}

	// 2. Commerce OrderLine WITHOUT Cost:
	lineNoCost := &commerce.OrderLine{
		Quantity:   1,
		UnitPrice:  money.FromMajor(90),
		TotalPrice: money.FromMajor(90),
		ListPrice:  money.FromMajor(100),
		CostPrice:  nil,
	}

	if lineNoCost.HasCostPrice() {
		t.Fatal("expected HasCostPrice to be false")
	}
	if lineNoCost.TotalCost().Minor() != 0 {
		t.Fatalf("expected 0 cost when no cost price, got %d", lineNoCost.TotalCost().Minor())
	}
	if lineNoCost.TotalNetProfit().Minor() != 0 {
		t.Fatalf("expected 0 profit when no cost price, got %d", lineNoCost.TotalNetProfit().Minor())
	}
}
