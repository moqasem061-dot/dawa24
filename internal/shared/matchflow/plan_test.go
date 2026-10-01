package matchflow_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestPlan_RequestSizingFromConfigLimits(t *testing.T) {
	// Generate 15 distinct questions
	var questions []matchflow.Question
	for i := 1; i <= 15; i++ {
		questions = append(questions, matchflow.Question{
			Key: fmt.Sprintf("q-%d", i),
			Item: matchflow.Item{
				Text: fmt.Sprintf("Product name %d with some extra details to consume tokens", i),
			},
			Window: []matchflow.CatalogEntry{
				{
					ProductID: int64(100 + i),
					NameAR:    fmt.Sprintf("منتج تجريبي رقم %d", i),
					NameEN:    fmt.Sprintf("Sample Product %d", i),
				},
			},
			Risk: 0.8,
		})
	}

	// 1. Test batch items capped by config.MaxItems (T136)
	c := matchflow.Ceilings{
		MaxRequestsPerRun: 10,
		Route:             "product.match.v8",
		Limits: matchflow.Limits{
			MaxItems: 5,
		},
	}
	reqs, _ := matchflow.Plan(questions, c)
	assert.NotEmpty(t, reqs)
	for _, req := range reqs {
		assert.LessOrEqual(t, len(req.Batch.Items), 5)
		assert.Equal(t, "product.match.v8", req.Batch.Route)
	}

	// 2. Test batch items fit by input tokens with bytes_per_token (T136)
	c2 := matchflow.Ceilings{
		MaxRequestsPerRun: 20,
		BytesPerToken:     2.0, // measured bytes per token
		MaxInputTokens:    120, // very tight token budget, so each batch can only fit a few items
		Limits: matchflow.Limits{
			MaxItems:       20,
			MaxInputTokens: 120,
		},
	}
	reqs2, _ := matchflow.Plan(questions, c2)
	assert.NotEmpty(t, reqs2)
	// Because tokens are tightly capped at 120 (with 2 bytes per token, ~240 bytes),
	// each request should hold fewer than 5 items
	for _, req := range reqs2 {
		assert.LessOrEqual(t, len(req.Batch.Items), 5)
	}
}
