package aicapabilities_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/aicapabilities"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// TestBlackHoleGatewayAssertsZeroUserFacingFailures points the gateway to an unroutable IP (RFC 5737 TEST-NET-1).
func TestBlackHoleGatewayAssertsZeroUserFacingFailures(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gwCfg := config.Gateway{
		BaseURL:    "http://192.0.2.1:1", // Unroutable black-hole IP
		VirtualKey: "dummy-key",
		ClientApp:  "dawa24_test",
		Enabled:    true,
		Timeout:    20 * time.Millisecond,
	}
	gwClient := gateway.New(gwCfg, logger)
	svc := aicapabilities.NewService(gwClient, logger)

	// 1. Match enhancement against a black hole returns an error, which every
	// importer treats as "no answer": the deterministic outcome stands. It
	// must never fall back to guessing — that is how a string-similarity
	// "match" used to reach compare files labelled as AI.
	id := int64(1)
	if _, err := svc.Enhance(ctx, matchflow.Batch{
		Catalog: []matchflow.CatalogEntry{{ProductID: 1, NameAR: "أوجمنتين 1 جم"}},
		Items:   []matchflow.Item{{Ref: 1, Text: "اوجمينتين 1 جم", Options: []int64{id}}},
	}); err == nil {
		t.Error("expected an error from an unreachable gateway, got an answer")
	}
	if _, err := svc.Verify(ctx, matchflow.CheckBatch{Pairs: []matchflow.Pair{{
		Ref: 1, Item: matchflow.Item{Text: "اوجمينتين 1 جم"},
		Product: matchflow.CatalogEntry{ProductID: 1, NameAR: "أوجمنتين 1 جم"},
	}}}); err == nil {
		t.Error("expected an error from an unreachable gateway on the pair check")
	}

	// 2. Search Expansion black-hole test
	expResp := svc.ExpandSearch(ctx, aicapabilities.QueryExpansionRequest{
		Query: "الكونجستال",
	})
	if expResp.Source != "deterministic_fallback" {
		t.Errorf("expected black-hole expansion to fallback gracefully, got source %q", expResp.Source)
	}
	if len(expResp.ExpandedTerms) == 0 {
		t.Errorf("expected expanded terms in fallback, got empty")
	}
}
