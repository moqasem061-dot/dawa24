package workflow_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestScoreDiscountLadder(t *testing.T) {
	tests := []struct {
		name       string
		price      string
		discount   string
		wantPoints float64
	}{
		{"50% or more", "100.00", "50.00", 30.0},
		{"30% to 49%", "100.00", "65.00", 25.0},
		{"20% to 29%", "100.00", "75.00", 20.0},
		{"10% to 19%", "100.00", "85.00", 15.0},
		{"5% to 9%", "100.00", "93.00", 10.0},
		{"1% to 4%", "100.00", "98.00", 5.0},
		{"0%", "100.00", "100.00", 0.0},
		{"no discount price", "100.00", "0.00", 0.0},
	}

	prefs := workflow.Priorities{
		PriorityHighestDiscount: true,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := workflow.CandidateProduct{
				ProductPrice:         money.MustParse(tc.price),
				ProductPriceDiscount: money.MustParse(tc.discount),
			}
			score, breakdown := workflow.Score(p, 10, 100, prefs)
			if score != tc.wantPoints {
				t.Errorf("got score %v, want %v", score, tc.wantPoints)
			}
			if breakdown["discount"] != tc.wantPoints {
				t.Errorf("got breakdown discount %v, want %v", breakdown["discount"], tc.wantPoints)
			}
		})
	}
}

func TestScorePriceRange(t *testing.T) {
	prefs := workflow.Priorities{
		PriorityLowestPrice: true,
	}

	// Range from 50 to 150 (range = 100)
	// Cheapest item (50) should get max 30 points
	// Mid item (100) should get (150-100)/100 * 30 = 15 points
	// Most expensive item (150) should get 0 points
	pLow := workflow.CandidateProduct{ProductPrice: money.MustParse("50.00")}
	pMid := workflow.CandidateProduct{ProductPrice: money.MustParse("100.00")}
	pHigh := workflow.CandidateProduct{ProductPrice: money.MustParse("150.00")}

	scoreLow, _ := workflow.Score(pLow, 50, 150, prefs)
	if scoreLow != 30.0 {
		t.Errorf("pLow score = %v, want 30.0", scoreLow)
	}

	scoreMid, _ := workflow.Score(pMid, 50, 150, prefs)
	if scoreMid != 15.0 {
		t.Errorf("pMid score = %v, want 15.0", scoreMid)
	}

	scoreHigh, _ := workflow.Score(pHigh, 50, 150, prefs)
	if scoreHigh != 0.0 {
		t.Errorf("pHigh score = %v, want 0.0", scoreHigh)
	}
}

func TestScoreDeliveryAndSupplier(t *testing.T) {
	prefs := workflow.Priorities{
		PriorityFastestDelivery:        true,
		PriorityPreferredSuppliersOnly: true,
		PreferredSupplierIDs:           []int64{10, 20},
	}

	p1 := workflow.CandidateProduct{
		OrganizationID:    10, // preferred
		EstimatedDelivery: 1,  // 25 pts delivery + 15 pts supplier = 40 pts
	}
	p2 := workflow.CandidateProduct{
		OrganizationID:    99, // not preferred
		EstimatedDelivery: 5,  // 10 pts delivery + 0 pts supplier = 10 pts
	}

	score1, bd1 := workflow.Score(p1, 0, 100, prefs)
	if score1 != 40.0 {
		t.Errorf("p1 score = %v, want 40.0", score1)
	}
	if bd1["delivery"] != 25.0 || bd1["supplier"] != 15.0 {
		t.Errorf("unexpected breakdown: %v", bd1)
	}

	score2, bd2 := workflow.Score(p2, 0, 100, prefs)
	if score2 != 10.0 {
		t.Errorf("p2 score = %v, want 10.0", score2)
	}
	if bd2["delivery"] != 10.0 || bd2["supplier"] != 0.0 {
		t.Errorf("unexpected breakdown: %v", bd2)
	}
}

func TestRankAndGenerateRecommendationsWithBudget(t *testing.T) {
	budget := money.MustParse("200.00")
	prefs := workflow.Priorities{
		PriorityHighestDiscount:        true,
		PriorityLowestPrice:            true,
		PriorityFastestDelivery:        true,
		PriorityPreferredSuppliersOnly: true,
		BudgetConstraint:               &budget,
		PreferredSupplierIDs:           []int64{1},
	}

	candidates := []workflow.CandidateProduct{
		{
			ProductID:            1,
			ProductName:          "Panadol Extra",
			ProductPrice:         money.MustParse("100.00"),
			ProductPriceDiscount: money.MustParse("50.00"), // 50% disc (30pt) + price (30pt) + del (25pt) + pref (15pt) = 100pt
			OrganizationID:       1,
			EstimatedDelivery:    1,
		},
		{
			ProductID:            2,
			ProductName:          "Augmentin 1g",
			ProductPrice:         money.MustParse("120.00"),
			ProductPriceDiscount: money.MustParse("90.00"),
			OrganizationID:       2,
			EstimatedDelivery:    2,
		},
		{
			ProductID:            3,
			ProductName:          "Expensive Item",
			ProductPrice:         money.MustParse("300.00"),
			ProductPriceDiscount: money.MustParse("250.00"), // Exceeds 200 budget
			OrganizationID:       3,
			EstimatedDelivery:    3,
		},
	}

	ranked := workflow.RankProducts(candidates, prefs)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked items, got %d", len(ranked))
	}
	if ranked[0].Product.ProductID != 1 {
		t.Errorf("expected product 1 to rank first, got %d", ranked[0].Product.ProductID)
	}

	recs := workflow.GenerateRecommendations(ranked, prefs)
	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations within budget, got %d", len(recs))
	}

	// First recommendation uses 50 EGP of 200 budget (rem = 150)
	if recs[0].BudgetImpact == nil || recs[0].BudgetImpact.RemainingAfter.String() != "150.00" {
		t.Errorf("unexpected budget impact on rec 0: %+v", recs[0].BudgetImpact)
	}
	// Second recommendation uses 90 EGP of 150 remaining budget (rem = 60)
	if recs[1].BudgetImpact == nil || recs[1].BudgetImpact.RemainingAfter.String() != "60.00" {
		t.Errorf("unexpected budget impact on rec 1: %+v", recs[1].BudgetImpact)
	}
}

func TestGeneratePurchasePriorityRequestNumberPattern(t *testing.T) {
	re := regexp.MustCompile(`^PPE-\d{4}-[A-Z0-9]{8}$`)
	for i := 0; i < 50; i++ {
		num := workflow.GeneratePurchasePriorityRequestNumber(time.Now().UTC())
		if !re.MatchString(num) {
			t.Fatalf("invalid request number format: %s", num)
		}
	}
}
