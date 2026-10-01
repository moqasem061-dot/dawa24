package matcheval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeAIMetrics_GainTermsAndPrecision(t *testing.T) {
	cases := []Case{
		{
			ID:    "c1",
			Label: Label{Resolvable: true, Targets: []int64{101}},
		},
		{
			ID:    "c2",
			Label: Label{Resolvable: true, Targets: []int64{102}},
		},
		{
			ID:    "c3",
			Label: Label{Resolvable: true, Targets: []int64{103}},
		},
		{
			ID:    "c4",
			Label: Label{Resolvable: true, Targets: []int64{104}},
		},
	}

	p101 := int64(101)
	p999 := int64(999)

	// c1: Det unresolved -> AI correct (GainNewCorrect)
	// c2: Det unresolved -> AI wrong (NewWrong)
	// c3: Det correct (103) -> AI wrong/abstain (LostCorrect)
	// c4: Det correct (104) -> AI correct (confirmation, zero gain)
	p103 := int64(103)
	p104 := int64(104)

	detOutcomes := map[string]Outcome{
		"c1": {CaseID: "c1", State: "needs_review"},
		"c2": {CaseID: "c2", State: "needs_review"},
		"c3": {CaseID: "c3", State: "accepted", MatchedProductID: &p103},
		"c4": {CaseID: "c4", State: "accepted", MatchedProductID: &p104},
	}

	aiOutcomes := map[string]Outcome{
		"c1": {CaseID: "c1", State: "accepted", MatchedProductID: &p101},
		"c2": {CaseID: "c2", State: "accepted", MatchedProductID: &p999},
		"c3": {CaseID: "c3", State: "needs_review"},
		"c4": {CaseID: "c4", State: "accepted", MatchedProductID: &p104},
	}

	stats := ComputeAIMetrics(cases, detOutcomes, aiOutcomes, 10, 500, 2.50, true)

	assert.Equal(t, 1, stats.GainNewCorrect)
	assert.Equal(t, 1, stats.NewWrong)
	assert.Equal(t, 1, stats.LostCorrect)
	// netGain = 1 - 1 - 1 = -1
	// resolvable = 4
	assert.Equal(t, -0.25, stats.NetGainRate)
	assert.Equal(t, 10, stats.Attempts)
	assert.Equal(t, 500, stats.Tokens)
	assert.Equal(t, 2.50, stats.Cost)
	assert.True(t, stats.CostKnown)
	assert.Equal(t, 2.50, stats.CostPerVerifiedGain)
	assert.Greater(t, stats.AIAddedPrecisionLB95, 0.0)
	assert.Equal(t, 1, stats.ZeroGainReasons["already_settled_confirmed"])
}
