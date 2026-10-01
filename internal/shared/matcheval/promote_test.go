package matcheval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromotePrompt_ResearchR16Rule(t *testing.T) {
	baseIncumbent := func() Report {
		return Report{
			Split: "tuning",
			Overall: CohortStats{
				AI: &AIStats{
					GainNewCorrect:       100,
					NewWrong:             5,
					LostCorrect:          1,
					AIAddedPrecisionLB95: 0.9200,
					Attempts:             10,
					Tokens:               1000, // 100 per attempt
				},
			},
		}
	}

	t.Run("promoted when all criteria pass", func(t *testing.T) {
		inc := baseIncumbent()
		cand := Report{
			Split: "tuning",
			Overall: CohortStats{
				AI: &AIStats{
					GainNewCorrect:       120, // Net gain: 120 - 4 - 0 = 116 > 94
					NewWrong:             4,
					LostCorrect:          0,
					AIAddedPrecisionLB95: 0.9300, // >= 0.9200
					Attempts:             10,
					Tokens:               1150, // 115 per attempt (+15% <= +20%)
				},
			},
		}

		ok, why := PromotePrompt(inc, cand)
		assert.True(t, ok)
		assert.Contains(t, why, "candidate qualified")
	})

	t.Run("rejected when net gain does not beat incumbent", func(t *testing.T) {
		inc := baseIncumbent() // Net gain: 94
		cand := Report{
			Split: "tuning",
			Overall: CohortStats{
				AI: &AIStats{
					GainNewCorrect:       95,
					NewWrong:             5,
					LostCorrect:          1, // Net gain: 89 <= 94
					AIAddedPrecisionLB95: 0.9300,
					Attempts:             10,
					Tokens:               1050,
				},
			},
		}

		ok, why := PromotePrompt(inc, cand)
		assert.False(t, ok)
		assert.Contains(t, why, "does not beat incumbent")
	})

	t.Run("rejected when precision lower bound is lowered", func(t *testing.T) {
		inc := baseIncumbent() // LB: 0.9200
		cand := Report{
			Split: "tuning",
			Overall: CohortStats{
				AI: &AIStats{
					GainNewCorrect:       130, // Net gain: 130 - 10 = 120 > 94
					NewWrong:             10,
					LostCorrect:          0,
					AIAddedPrecisionLB95: 0.8900, // Lower than 0.9200!
					Attempts:             10,
					Tokens:               1100,
				},
			},
		}

		ok, why := PromotePrompt(inc, cand)
		assert.False(t, ok)
		assert.Contains(t, why, "precision lower bound")
	})

	t.Run("rejected when tokens increase by more than 20 percent", func(t *testing.T) {
		inc := baseIncumbent() // 100 tokens per attempt
		cand := Report{
			Split: "tuning",
			Overall: CohortStats{
				AI: &AIStats{
					GainNewCorrect:       120, // Net gain: 116 > 94
					NewWrong:             4,
					LostCorrect:          0,
					AIAddedPrecisionLB95: 0.9300,
					Attempts:             10,
					Tokens:               1350, // 135 tokens per attempt (+35% > 20% limit!)
				},
			},
		}

		ok, why := PromotePrompt(inc, cand)
		assert.False(t, ok)
		assert.Contains(t, why, "20% limit")
	})

	t.Run("rejected when missing AI stats", func(t *testing.T) {
		inc := baseIncumbent()
		cand := Report{Split: "tuning"}

		ok, why := PromotePrompt(inc, cand)
		assert.False(t, ok)
		assert.Contains(t, why, "missing AI")
	})
}
