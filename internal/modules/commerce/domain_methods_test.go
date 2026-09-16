package commerce_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestOrderLine_CostAndProfitCalculations(t *testing.T) {
	t.Run("Line with explicit CostPrice and CostDiscountPercentage", func(t *testing.T) {
		costPrice := money.FromMajor(50)
		line := &commerce.OrderLine{
			Quantity:               3,
			UnitPrice:              money.FromMajor(80),
			TotalPrice:             money.FromMajor(240), // 3 * 80
			ListPrice:              money.FromMajor(100), // Retail strike price 100
			CostPrice:              &costPrice,
			CostDiscountPercentage: 25.0,                  // 25% cost discount on public price (100) -> 75 unit cost
		}

		if !line.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be true")
		}

		// Unit discounted purchase cost on public price: 100 - (100 * 0.25) = 75 EGP = 7500 minor
		unitCost := line.EffectivePurchaseCost()
		if unitCost.Minor() != 7500 {
			t.Fatalf("expected unit cost 7500, got %d", unitCost.Minor())
		}

		// Total purchase cost: 75 * 3 = 225 EGP = 22500 minor
		totPurchCost := line.TotalPurchaseCost()
		if totPurchCost.Minor() != 22500 {
			t.Fatalf("expected total purchase cost 22500, got %d", totPurchCost.Minor())
		}

		// TotalCost: must strictly be purchase cost (22500 minor).
		// Selling discount must NOT be added!
		totCost := line.TotalCost()
		if totCost.Minor() != 22500 {
			t.Fatalf("expected total cost 22500, got %d (selling discount must not be added)", totCost.Minor())
		}

		// Net profit: TotalPrice (240) - TotalCost (225) = 15 EGP = 1500 minor
		netProfit := line.TotalNetProfit()
		if netProfit.Minor() != 1500 {
			t.Fatalf("expected net profit 1500, got %d", netProfit.Minor())
		}
	})

	t.Run("Line with explicit CostPrice and zero CostDiscountPercentage", func(t *testing.T) {
		costPrice := money.FromMajor(50)
		line := &commerce.OrderLine{
			Quantity:               3,
			UnitPrice:              money.FromMajor(80),
			TotalPrice:             money.FromMajor(240),
			ListPrice:              money.FromMajor(100),
			CostPrice:              &costPrice,
			CostDiscountPercentage: 0.0,
		}

		unitCost := line.EffectivePurchaseCost()
		if unitCost.Minor() != 5000 {
			t.Fatalf("expected unit cost 5000, got %d", unitCost.Minor())
		}

		totCost := line.TotalCost()
		if totCost.Minor() != 15000 {
			t.Fatalf("expected total cost 15000, got %d", totCost.Minor())
		}

		netProfit := line.TotalNetProfit()
		if netProfit.Minor() != 9000 {
			t.Fatalf("expected net profit 9000, got %d", netProfit.Minor())
		}
	})

	t.Run("Line without CostPrice results in 0 cost and 0 net profit", func(t *testing.T) {
		line := &commerce.OrderLine{
			Quantity:   2,
			UnitPrice:  money.FromMajor(80),
			TotalPrice: money.FromMajor(160),
			ListPrice:  money.FromMajor(100),
			CostPrice:  nil, // No cost recorded
		}

		if line.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be false")
		}

		if line.EffectivePurchaseCost().Minor() != 0 {
			t.Fatalf("expected 0 effective purchase cost, got %d", line.EffectivePurchaseCost().Minor())
		}

		if line.TotalPurchaseCost().Minor() != 0 {
			t.Fatalf("expected 0 total purchase cost, got %d", line.TotalPurchaseCost().Minor())
		}

		if line.TotalCost().Minor() != 0 {
			t.Fatalf("expected 0 total cost, got %d", line.TotalCost().Minor())
		}

		if line.TotalNetProfit().Minor() != 0 {
			t.Fatalf("expected 0 net profit, got %d", line.TotalNetProfit().Minor())
		}
	})
}
