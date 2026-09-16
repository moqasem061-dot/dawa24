package workflow

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// InstitutionalGate resolves authorized institutional works for a user.
type InstitutionalGate interface {
	AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error)
}

// InstitutionalGateFunc adapts a function signature to the InstitutionalGate interface.
type InstitutionalGateFunc func(ctx context.Context, userID int64, mode int) ([]int64, error)

// AllowedWorkIDs implements InstitutionalGate.
func (f InstitutionalGateFunc) AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error) {
	return f(ctx, userID, mode)
}

// SetInstitutionalGate sets the gate dependency on workflow service.
func (s *Service) SetInstitutionalGate(g InstitutionalGate) {
	s.instGate = g
}

// CreatePriorityEngine creates a new purchase priority engine run (Plan V5 Phase 3 §3.2).
func (s *Service) CreatePriorityEngine(ctx context.Context, userID int64, orgID *int64, prefs Priorities) (*PurchasePriorityRequest, error) {
	if userID <= 0 {
		return nil, apperr.Validation("priority_engine.user_required", "User ID is required.", nil)
	}

	req := &PurchasePriorityRequest{
		UserID:                         userID,
		OrganizationID:                 orgID,
		RequestNumber:                  GeneratePurchasePriorityRequestNumber(time.Now().UTC()),
		Status:                         "pending",
		PriorityHighestDiscount:        prefs.PriorityHighestDiscount,
		PriorityLowestPrice:            prefs.PriorityLowestPrice,
		PriorityFastestDelivery:        prefs.PriorityFastestDelivery,
		PriorityPreferredSuppliersOnly: prefs.PriorityPreferredSuppliersOnly,
	}
	if prefs.BudgetConstraint != nil && prefs.BudgetConstraint.IsPositive() {
		req.BudgetConstraint = *prefs.BudgetConstraint
	}

	if err := s.repo.CreatePriorityRequest(ctx, req); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "purchase priority engine created", "id", req.ID, "request_number", req.RequestNumber, "user_id", userID)
	return req, nil
}

// ProcessPriorityEngine runs the complete ranking and recommendation pipeline for a priority engine task.
func (s *Service) ProcessPriorityEngine(ctx context.Context, engineID int64, processedBy *int64) (*ProcessingSummary, error) {
	req, err := s.repo.GetPriorityRequestByID(ctx, engineID)
	if err != nil {
		return nil, err
	}

	// 1. Mark as processing
	_ = s.repo.UpdatePriorityRequestStatus(ctx, engineID, "processing", "", processedBy, nil)

	startTime := time.Now()

	// 2. Fetch authorized institutional works (Simple mode 0)
	var authorizedWorkIDs []int64
	if s.instGate != nil {
		works, err := s.instGate.AllowedWorkIDs(ctx, req.UserID, 0)
		if err == nil {
			authorizedWorkIDs = works
		}
	}

	// 3. Build query parameters & preferences
	var budgetPtr *money.Amount
	if req.BudgetConstraint.IsPositive() {
		b := req.BudgetConstraint
		budgetPtr = &b
	}

	prefs := Priorities{
		PriorityHighestDiscount:        req.PriorityHighestDiscount,
		PriorityLowestPrice:            req.PriorityLowestPrice,
		PriorityFastestDelivery:        req.PriorityFastestDelivery,
		PriorityPreferredSuppliersOnly: req.PriorityPreferredSuppliersOnly,
		BudgetConstraint:               budgetPtr,
	}

	// 4. Fetch candidate products from read model
	candidates, err := s.repo.GetCandidateProducts(ctx, req.UserID, authorizedWorkIDs, prefs.PreferredSupplierIDs, budgetPtr, 1000)
	if err != nil {
		s.log.ErrorContext(ctx, "failed fetching candidate products for priority engine", "id", engineID, "error", err)
		_ = s.repo.UpdatePriorityRequestStatus(ctx, engineID, "failed", err.Error(), processedBy, nil)
		return nil, err
	}

	// 5. Apply AI scoring and ranking
	ranked := RankProducts(candidates, prefs)

	// 6. Generate recommendations respecting budget constraint
	recommendations := GenerateRecommendations(ranked, prefs)

	// 7. Calculate summary
	durationMs := time.Since(startTime).Milliseconds()
	avgScore := 0.0
	if len(recommendations) > 0 {
		sum := 0.0
		for _, r := range recommendations {
			sum += r.Score
		}
		avgScore = sum / float64(len(recommendations))
	}

	summary := &ProcessingSummary{
		TotalProductsAnalyzed:    len(candidates),
		RecommendationsGenerated: len(recommendations),
		PrioritiesApplied:        formatPrioritiesApplied(prefs),
		BudgetConstraint:         budgetPtr,
		ProcessingDurationMs:     durationMs,
		AverageScore:             avgScore,
	}

	results := map[string]any{
		"matched_products_count": len(candidates),
		"ranked_products":        ranked,
		"recommendations":        recommendations,
		"processing_summary":     summary,
	}

	// 8. Mark as completed
	if err := s.repo.UpdatePriorityRequestStatus(ctx, engineID, "completed", "", processedBy, results); err != nil {
		s.log.ErrorContext(ctx, "failed updating completed priority engine", "id", engineID, "error", err)
		return nil, err
	}

	s.log.InfoContext(ctx, "purchase priority engine completed", "id", engineID, "analyzed", len(candidates), "recommendations", len(recommendations))
	return summary, nil
}

// GetPriorityRequest retrieves a priority engine request by its ID.
func (s *Service) GetPriorityRequest(ctx context.Context, id int64) (*PurchasePriorityRequest, error) {
	return s.repo.GetPriorityRequestByID(ctx, id)
}

// ListPriorityEngines lists past priority engine runs for a user.
func (s *Service) ListPriorityEngines(ctx context.Context, userID int64, limit, offset int) ([]*PurchasePriorityRequest, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListPriorityRequestsByUser(ctx, userID, limit, offset)
}

func formatPrioritiesApplied(p Priorities) []string {
	var applied []string
	if p.PriorityHighestDiscount {
		applied = append(applied, i18n.TDefault("w4_mod.highest_discount_257"))
	}
	if p.PriorityLowestPrice {
		applied = append(applied, i18n.TDefault("w4_mod.lowest_price_258"))
	}
	if p.PriorityFastestDelivery {
		applied = append(applied, i18n.TDefault("w4_mod.fastest_delivery_259"))
	}
	if p.PriorityPreferredSuppliersOnly {
		applied = append(applied, i18n.TDefault("w4_mod.preferred_suppliers_only_260"))
	}
	if p.BudgetConstraint != nil && p.BudgetConstraint.IsPositive() {
		applied = append(applied, fmt.Sprintf("Budget: %s EGP", p.BudgetConstraint.String()))
	}
	return applied
}
