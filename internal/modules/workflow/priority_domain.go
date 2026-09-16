package workflow

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CandidateProduct represents a product evaluated by the Purchase Priority Engine.
type CandidateProduct struct {
	ProductID            int64        `json:"product_id"`
	ProductName          string       `json:"product_name"`
	ProductPrice         money.Amount `json:"product_price"`
	ProductPriceDiscount money.Amount `json:"product_price_discount"`
	ProductSKU           string       `json:"product_sku,omitempty"`
	ProductBarcode       string       `json:"product_barcode,omitempty"`
	OrganizationID       int64        `json:"organization_id"`
	BranchID             *int64       `json:"branch_id,omitempty"`
	StockQuantity        int          `json:"stock_quantity"`
	ParentProductName    string       `json:"parent_product_name,omitempty"`
	OrganizationName     string       `json:"organization_name,omitempty"`
	BranchName           string       `json:"branch_name,omitempty"`
	InstitutionalWorkIDs []int64      `json:"institutional_work_ids,omitempty"`
	EstimatedDelivery    int          `json:"estimated_delivery"` // in days
}

// FinalPrice returns the effective price after discount if positive, otherwise original price.
func (c *CandidateProduct) FinalPrice() money.Amount {
	if c.ProductPriceDiscount.IsPositive() {
		return c.ProductPriceDiscount
	}
	return c.ProductPrice
}

// DiscountPercentage returns the discount percentage relative to original price (0-100).
func (c *CandidateProduct) DiscountPercentage() float64 {
	if !c.ProductPriceDiscount.IsPositive() || !c.ProductPrice.IsPositive() {
		return 0
	}
	diff := c.ProductPrice.Minor() - c.ProductPriceDiscount.Minor()
	if diff <= 0 {
		return 0
	}
	return math.Round((float64(diff)/float64(c.ProductPrice.Minor()))*10000) / 100
}

// Priorities represents the configuration and criteria for priority ranking.
type Priorities struct {
	PriorityHighestDiscount        bool          `json:"priority_highest_discount"`
	PriorityLowestPrice            bool          `json:"priority_lowest_price"`
	PriorityFastestDelivery        bool          `json:"priority_fastest_delivery"`
	PriorityPreferredSuppliersOnly bool          `json:"priority_preferred_suppliers_only"`
	BudgetConstraint               *money.Amount `json:"budget_constraint,omitempty"`
	PreferredSupplierIDs           []int64       `json:"preferred_supplier_ids,omitempty"`
}

// ScoredProduct holds the calculated ranking score and breakdown for a candidate product.
type ScoredProduct struct {
	Product             CandidateProduct   `json:"product"`
	TotalScore          float64            `json:"total_score"`
	ScoreBreakdown      map[string]float64 `json:"score_breakdown"`
	FinalPrice          money.Amount       `json:"final_price"`
	DiscountPercentage  float64            `json:"discount_percentage"`
	EstimatedDelivery   int                `json:"estimated_delivery"`
	IsPreferredSupplier bool               `json:"is_preferred_supplier"`
}

// BudgetImpact captures the financial effect on the total remaining budget.
type BudgetImpact struct {
	Price              money.Amount `json:"price"`
	RemainingAfter     money.Amount `json:"remaining_after"`
	PercentageOfBudget float64      `json:"percentage_of_budget"`
}

// Recommendation represents a final recommended product selection.
type Recommendation struct {
	Rank                 int                `json:"rank"`
	Product              CandidateProduct   `json:"product"`
	Score                float64            `json:"score"`
	ScoreBreakdown       map[string]float64 `json:"score_breakdown"`
	FinalPrice           money.Amount       `json:"final_price"`
	DiscountPercentage   float64            `json:"discount_percentage"`
	EstimatedDelivery    int                `json:"estimated_delivery"`
	IsPreferredSupplier  bool               `json:"is_preferred_supplier"`
	RecommendationReason string             `json:"recommendation_reason"`
	BudgetImpact         *BudgetImpact      `json:"budget_impact,omitempty"`
}

// ProcessingSummary holds metrics and statistics about the execution run.
type ProcessingSummary struct {
	TotalProductsAnalyzed    int           `json:"total_products_analyzed"`
	RecommendationsGenerated int           `json:"recommendations_generated"`
	PrioritiesApplied        []string      `json:"priorities_applied"`
	BudgetConstraint         *money.Amount `json:"budget_constraint,omitempty"`
	ProcessingDurationMs     int64         `json:"processing_duration_ms"`
	AverageScore             float64       `json:"average_score"`
}

// Score ranks one candidate product against the requester's priorities.
// The weights reproduce PurchasePriorityEngineService::applyAIRanking exactly.
func Score(p CandidateProduct, minPrice, maxPrice float64, prefs Priorities) (float64, map[string]float64) {
	score := 0.0
	breakdown := make(map[string]float64)

	// 1. Discount scoring (0-30 points)
	if prefs.PriorityHighestDiscount {
		discPct := p.DiscountPercentage()
		var dScore float64
		if discPct >= 50 {
			dScore = 30
		} else if discPct >= 30 {
			dScore = 25
		} else if discPct >= 20 {
			dScore = 20
		} else if discPct >= 10 {
			dScore = 15
		} else if discPct >= 5 {
			dScore = 10
		} else if discPct > 0 {
			dScore = 5
		}
		score += dScore
		breakdown["discount"] = dScore
	}

	// 2. Price scoring (0-30 points)
	if prefs.PriorityLowestPrice {
		finalPriceMajor := float64(p.FinalPrice().Minor()) / 100.0
		var pScore float64
		if maxPrice == minPrice {
			pScore = 15.0
		} else {
			priceRange := maxPrice - minPrice
			if priceRange > 0 {
				pos := (maxPrice - finalPriceMajor) / priceRange
				pScore = math.Round(pos*3000) / 100.0
			}
		}
		score += pScore
		breakdown["price"] = pScore
	}

	// 3. Delivery scoring (0-25 points)
	if prefs.PriorityFastestDelivery {
		delDays := p.EstimatedDelivery
		if delDays <= 0 {
			delDays = 1 // default 1 day
		}
		var delScore float64
		if delDays <= 1 {
			delScore = 25
		} else if delDays <= 2 {
			delScore = 20
		} else if delDays <= 3 {
			delScore = 15
		} else if delDays <= 5 {
			delScore = 10
		} else if delDays <= 7 {
			delScore = 5
		}
		score += delScore
		breakdown["delivery"] = delScore
	}

	// 4. Supplier preference scoring (0-15 points)
	if prefs.PriorityPreferredSuppliersOnly {
		isPref := isPreferred(p.OrganizationID, prefs.PreferredSupplierIDs)
		var supScore float64
		if isPref {
			supScore = 15
		}
		score += supScore
		breakdown["supplier"] = supScore
	}

	totalScore := math.Round(score*100) / 100
	return totalScore, breakdown
}

// RankProducts scores and sorts candidates descending by total score.
func RankProducts(candidates []CandidateProduct, prefs Priorities) []ScoredProduct {
	if len(candidates) == 0 {
		return nil
	}

	// Find min and max price among candidates
	minPrice := float64(candidates[0].FinalPrice().Minor()) / 100.0
	maxPrice := minPrice
	for _, c := range candidates[1:] {
		p := float64(c.FinalPrice().Minor()) / 100.0
		if p < minPrice {
			minPrice = p
		}
		if p > maxPrice {
			maxPrice = p
		}
	}

	scored := make([]ScoredProduct, 0, len(candidates))
	for _, c := range candidates {
		totScore, breakdown := Score(c, minPrice, maxPrice, prefs)
		isPref := isPreferred(c.OrganizationID, prefs.PreferredSupplierIDs)
		del := c.EstimatedDelivery
		if del <= 0 {
			del = 1
		}
		scored = append(scored, ScoredProduct{
			Product:             c,
			TotalScore:          totScore,
			ScoreBreakdown:      breakdown,
			FinalPrice:          c.FinalPrice(),
			DiscountPercentage:  c.DiscountPercentage(),
			EstimatedDelivery:   del,
			IsPreferredSupplier: isPref,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].TotalScore > scored[j].TotalScore
	})

	return scored
}

// GenerateRecommendations produces top <= 20 recommendations respecting budget constraints.
func GenerateRecommendations(ranked []ScoredProduct, prefs Priorities) []Recommendation {
	var recs []Recommendation
	var remainingBudget *money.Amount
	var totalBudget *money.Amount

	if prefs.BudgetConstraint != nil && prefs.BudgetConstraint.IsPositive() {
		b := *prefs.BudgetConstraint
		totalBudget = &b
		rem := b
		remainingBudget = &rem
	}

	for idx, r := range ranked {
		if remainingBudget != nil {
			if r.FinalPrice.Minor() > remainingBudget.Minor() {
				continue
			}
		}

		var impact *BudgetImpact
		if totalBudget != nil && remainingBudget != nil {
			remAfter, _ := remainingBudget.Sub(r.FinalPrice)
			pct := 0.0
			if totalBudget.Minor() > 0 {
				pct = math.Round((float64(r.FinalPrice.Minor())/float64(totalBudget.Minor()))*10000) / 100
			}
			impact = &BudgetImpact{
				Price:              r.FinalPrice,
				RemainingAfter:     remAfter,
				PercentageOfBudget: pct,
			}
		}

		rec := Recommendation{
			Rank:                 idx + 1,
			Product:              r.Product,
			Score:                r.TotalScore,
			ScoreBreakdown:       r.ScoreBreakdown,
			FinalPrice:           r.FinalPrice,
			DiscountPercentage:   r.DiscountPercentage,
			EstimatedDelivery:    r.EstimatedDelivery,
			IsPreferredSupplier:  r.IsPreferredSupplier,
			RecommendationReason: GenerateRecommendationReason(r, prefs),
			BudgetImpact:         impact,
		}

		recs = append(recs, rec)

		if remainingBudget != nil {
			rem, _ := remainingBudget.Sub(r.FinalPrice)
			remainingBudget = &rem
		}

		if len(recs) >= 20 {
			break
		}
	}

	return recs
}

// GenerateRecommendationReason explains why a product was recommended.
func GenerateRecommendationReason(r ScoredProduct, prefs Priorities) string {
	var reasons []string
	bd := r.ScoreBreakdown

	if prefs.PriorityHighestDiscount && bd["discount"] > 20 {
		reasons = append(reasons, fmt.Sprintf("Excellent discount (%.1f%%)", r.DiscountPercentage))
	}
	if prefs.PriorityLowestPrice && bd["price"] > 20 {
		reasons = append(reasons, "Competitive pricing")
	}
	if prefs.PriorityFastestDelivery && bd["delivery"] > 15 {
		reasons = append(reasons, fmt.Sprintf("Fast delivery (%d days)", r.EstimatedDelivery))
	}
	if prefs.PriorityPreferredSuppliersOnly && r.IsPreferredSupplier {
		reasons = append(reasons, "Preferred supplier")
	}

	if len(reasons) == 0 {
		return "Best overall match"
	}
	return strings.Join(reasons, "; ")
}

// GeneratePurchasePriorityRequestNumber formats PPE-YYYY-RANDOM8 matching Laravel's pattern (Rule R7).
func GeneratePurchasePriorityRequestNumber(t time.Time) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return fmt.Sprintf("PPE-%s-%s", t.Format("2006"), string(b))
}

func isPreferred(orgID int64, preferredIDs []int64) bool {
	for _, id := range preferredIDs {
		if id == orgID {
			return true
		}
	}
	return false
}
