package matching

import (
	"context"
	"math/rand"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// FlagAIDisagrees is added to a row's decision flags when audit mode AI disagrees with the accepted target (US9 T130).
const FlagAIDisagrees = "ai_disagrees"

// SampleAuditRows selects a random sample of settled/accepted rows according to auditRate (0.0 to 1.0).
func SampleAuditRows(rows []EnhanceRow, auditRate float64, r *rand.Rand) []EnhanceRow {
	if auditRate <= 0 || len(rows) == 0 {
		return nil
	}
	if auditRate > 1.0 {
		auditRate = 1.0
	}
	if r == nil {
		r = rand.New(rand.NewSource(42))
	}

	var sampled []EnhanceRow
	for _, row := range rows {
		if row.Decision.State != matchflow.StateAccepted && row.Decision.State != matchflow.StateConfirmed {
			continue
		}
		if row.Decision.ProductID == nil {
			continue
		}
		if r.Float64() < auditRate {
			sampled = append(sampled, row)
		}
	}
	return sampled
}

// AuditAuditor checks sampled accepted rows with an AI enhancer.
// When the AI disagrees or flags a concern, it sets the ai_disagrees flag
// without changing the accepted target product ID (US9 T130).
type AuditAuditor struct {
	enhancer matchflow.Enhancer
}

// NewAuditAuditor constructs an AuditAuditor.
func NewAuditAuditor(enhancer matchflow.Enhancer) *AuditAuditor {
	return &AuditAuditor{enhancer: enhancer}
}

// AuditRows executes audit evaluation on the sampled rows and applies ai_disagrees flags.
func (a *AuditAuditor) AuditRows(
	ctx context.Context,
	runID int64,
	rows []EnhanceRow,
	applier Applier,
) (int, error) {
	if a.enhancer == nil || len(rows) == 0 || applier == nil {
		return 0, nil
	}

	var disagreements int
	for _, er := range rows {
		targetID := *er.Decision.ProductID

		// Build a single-item batch to verify the accepted match
		batch := matchflow.Batch{
			Items: []matchflow.Item{
				{
					Ref:          0,
					Text:         er.Row.Name,
					CurrentGuess: &targetID,
					Settled:      true,
				},
			},
		}

		decisions, err := a.enhancer.Enhance(ctx, batch)
		if err != nil || len(decisions) == 0 {
			continue
		}

		dec := decisions[0]
		// Concern is indicated when AI explicitly returns nil (none of these) or a different product
		if dec.ProductID == nil || *dec.ProductID != targetID {
			updatedDec := er.Decision
			hasFlag := false
			for _, f := range updatedDec.Flags {
				if f == FlagAIDisagrees {
					hasFlag = true
					break
				}
			}
			if !hasFlag {
				updatedDec.Flags = append(updatedDec.Flags, FlagAIDisagrees)
			}

			applied, aErr := applier.ApplyAI(ctx, er.RowRef, er.Decision.Revision, runID, updatedDec)
			if aErr == nil && applied {
				disagreements++
			}
		}
	}

	return disagreements, nil
}
