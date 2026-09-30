package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestDefaultPolicy(t *testing.T) {
	pol := productmatch.DefaultPolicy()

	if pol.MinStrong != 0.60 {
		t.Errorf("expected MinStrong = 0.60, got %f", pol.MinStrong)
	}
	if pol.MinReview != 0.25 {
		t.Errorf("expected MinReview = 0.25, got %f", pol.MinReview)
	}
	if pol.TieMargin != 0.06 {
		t.Errorf("expected TieMargin = 0.06, got %f", pol.TieMargin)
	}
	if pol.BlindNearIdentical != 0.90 {
		t.Errorf("expected BlindNearIdentical = 0.90, got %f", pol.BlindNearIdentical)
	}
	if pol.Required != productmatch.GranularityProduct {
		t.Errorf("expected Required = GranularityProduct, got %v", pol.Required)
	}
	if !pol.AllowOmittedAttrs {
		t.Errorf("expected AllowOmittedAttrs = true, got false")
	}

	// All identifier tiers off by default
	if pol.Identifiers.AllowGTIN {
		t.Errorf("expected AllowGTIN = false, got true")
	}
	if pol.Identifiers.AllowCatalogCode {
		t.Errorf("expected AllowCatalogCode = false, got true")
	}
	if pol.Identifiers.AllowSupplierCode {
		t.Errorf("expected AllowSupplierCode = false, got true")
	}
}

func TestGranularity(t *testing.T) {
	tests := []struct {
		g           productmatch.Granularity
		wantStr     string
		wantFlowVal matchflow.Granularity
	}{
		{productmatch.GranularityNone, "none", matchflow.GranularityNone},
		{productmatch.GranularityProduct, "product", matchflow.GranularityProduct},
		{productmatch.GranularityPack, "pack", matchflow.GranularityPack},
		{productmatch.Granularity(99), "none", matchflow.GranularityNone},
	}

	for _, tt := range tests {
		if tt.g.String() != tt.wantStr {
			t.Errorf("Granularity(%d).String() = %q, want %q", tt.g, tt.g.String(), tt.wantStr)
		}
		if tt.g.MatchflowGranularity() != tt.wantFlowVal {
			t.Errorf("Granularity(%d).MatchflowGranularity() = %v, want %v", tt.g, tt.g.MatchflowGranularity(), tt.wantFlowVal)
		}
	}
}
