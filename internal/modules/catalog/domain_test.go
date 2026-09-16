package catalog_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestProductVariant_CostAndProfitCalculations(t *testing.T) {
	t.Run("Variant with cost and discounts", func(t *testing.T) {
		cost := money.FromMajor(80)
		v := &catalog.ProductVariant{
			Price:                  money.FromMajor(100), // Retail price
			Discount:               money.FromMinor(1000), // 10.00% selling discount -> 90 selling price
			CostPrice:              &cost,
			CostDiscountPercentage: 20.0, // 20% cost discount -> 80 - 16 = 64
		}

		if !v.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be true")
		}

		effSell := v.EffectiveSellingPrice()
		if effSell.Minor() != 9000 {
			t.Fatalf("expected effective selling price 9000 (90 EGP), got %d", effSell.Minor())
		}

		discCost := v.DiscountedCost()
		// Egyptian pharma: Cost discount (20%) is applied on Public Price (100) -> 100 - 20 = 80 EGP = 8000 minor
		if discCost.Minor() != 8000 {
			t.Fatalf("expected discounted cost 8000 (80 EGP), got %d", discCost.Minor())
		}

		// UnitTotalCost must strictly equal DiscountedCost (80), NOT adding selling discount (10)
		totCost := v.UnitTotalCost()
		if totCost.Minor() != 8000 {
			t.Fatalf("expected unit total cost 8000, got %d", totCost.Minor())
		}

		// Net profit: 90 - 80 = 10 EGP = 1000 minor
		netProfit := v.UnitNetProfit()
		if netProfit.Minor() != 1000 {
			t.Fatalf("expected net profit 1000 (10 EGP), got %d", netProfit.Minor())
		}

		// Margin %: (10 / 90) * 100 = 11.111...%
		margin := v.ProfitMarginPercent()
		expectedMargin := (10.0 / 90.0) * 100.0
		if margin < expectedMargin-0.01 || margin > expectedMargin+0.01 {
			t.Fatalf("expected margin ~%.2f%%, got %.2f%%", expectedMargin, margin)
		}
	})

	t.Run("Variant with CostDiscountPercentage but nil CostPrice", func(t *testing.T) {
		v := &catalog.ProductVariant{
			Price:                  money.FromMajor(100), // Retail price
			Discount:               money.FromMinor(2000), // 20% selling discount -> 80
			CostDiscountPercentage: 25.0,                  // 25% cost discount on public price -> 75
		}

		if !v.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be true when CostDiscountPercentage > 0")
		}

		discCost := v.DiscountedCost()
		if discCost.Minor() != 7500 {
			t.Fatalf("expected discounted cost 7500 (75 EGP), got %d", discCost.Minor())
		}

		netProfit := v.UnitNetProfit()
		if netProfit.Minor() != 500 {
			t.Fatalf("expected net profit 500 (5 EGP), got %d", netProfit.Minor())
		}
	})

	t.Run("Variant without cost price returns 0 cost and 0.0% margin", func(t *testing.T) {
		v := &catalog.ProductVariant{
			Price:    money.FromMajor(100),
			Discount: money.FromMinor(1000), // 10%
		}

		if v.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be false")
		}

		if v.DiscountedCost().Minor() != 0 {
			t.Fatalf("expected 0 discounted cost when no cost price, got %d", v.DiscountedCost().Minor())
		}

		if v.UnitTotalCost().Minor() != 0 {
			t.Fatalf("expected 0 total cost when no cost price, got %d", v.UnitTotalCost().Minor())
		}

		if v.UnitNetProfit().Minor() != 0 {
			t.Fatalf("expected 0 net profit when no cost price, got %d", v.UnitNetProfit().Minor())
		}

		margin := v.ProfitMarginPercent()
		if margin != 0.0 {
			t.Fatalf("expected exactly 0.0%% margin when no cost price, got %.2f%%", margin)
		}
	})
}
