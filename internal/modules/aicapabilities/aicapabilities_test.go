package aicapabilities_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

func TestDeterministicFallbackWhenGatewayDisabled(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gwCfg := config.Gateway{
		BaseURL:    "http://localhost:8080",
		VirtualKey: "dummy-key",
		ClientApp:  "dawa24_test",
		Enabled:    false, // Explicitly disabled
		Timeout:    50 * time.Millisecond,
	}
	gwClient := gateway.New(gwCfg, logger)
	svc := aicapabilities.NewService(gwClient, logger)

	req := aicapabilities.MatchRequest{
		QueryName: "بانادول اكسترا اقراص",
		Candidates: []string{
			"بنادول اكسترا اقراص",
			"كتافلام 50 مجم",
			"كونجستال اقراص",
		},
	}

	resp := svc.MatchProduct(ctx, req)
	if resp.Source != "deterministic_fallback" {
		t.Errorf("expected source = 'deterministic_fallback', got %q", resp.Source)
	}
	if resp.MatchedCandidate != "بنادول اكسترا اقراص" {
		t.Errorf("expected matched candidate = 'بنادول اكسترا اقراص', got %q", resp.MatchedCandidate)
	}
	if resp.ConfidenceScore < 0.85 {
		t.Errorf("expected confidence score >= 0.85, got %f", resp.ConfidenceScore)
	}
}

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

	// 1. Product Matching black-hole test
	matchResp := svc.MatchProduct(ctx, aicapabilities.MatchRequest{
		QueryName:  "اوجمينتين 1 جم",
		Candidates: []string{"أوجمنتين 1 جم", "فولتارين 75"},
	})
	if matchResp.Source != "deterministic_fallback" {
		t.Errorf("expected black-hole match to fallback gracefully, got source %q", matchResp.Source)
	}
	if matchResp.MatchedCandidate != "أوجمنتين 1 جم" {
		t.Errorf("unexpected matched candidate: %q", matchResp.MatchedCandidate)
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
