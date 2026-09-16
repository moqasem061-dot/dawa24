package integration_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The decision cache is shared or it is nothing.
//
// Its whole economic argument is that an answer paid for by a pharmacy's order
// is free to the vendor whose price list asks the same question, and the
// reverse. Two things have to hold for that: the prompt version has to match,
// and the key has to be computed identically.
//
// Neither held. ingest declared "sm-enh-v3" and smartorder "sm-enh-v4" — 2,913
// cached answers on one side and 3,584 on the other, for one question, neither
// ever reading the other's — under a comment asserting they matched. And the two
// key functions were byte-identical copies in files nobody diffs, which is not
// the same as being the same.
//
// These tests live in test/integration rather than in either module because
// neither module may import the other. That constraint is exactly what let them
// drift, so the assertion belongs where both are visible at once.

// TestPromptVersionIsShared guards the constant.
func TestPromptVersionIsShared(t *testing.T) {
	if ingest.PromptVersion != matchflow.PromptVersion {
		t.Errorf("vendor import prompt version %q != shared %q",
			ingest.PromptVersion, matchflow.PromptVersion)
	}
	if pipeline.PromptVersion != matchflow.PromptVersion {
		t.Errorf("smart order prompt version %q != shared %q",
			pipeline.PromptVersion, matchflow.PromptVersion)
	}
}

// TestBothImportersProduceTheSameCacheKey is the assertion that actually
// matters: the same medicine, retrieved against the same catalogue products,
// must hash to one row of catalog.match_decisions whichever importer asked.
//
// It drives the two modules' own key functions through their real row types
// rather than re-deriving the key here, because a test that recomputes the thing
// it is checking proves nothing.
func TestBothImportersProduceTheSameCacheKey(t *testing.T) {
	const raw = "أوجمنتين 1 جم 14 قرص"
	candidates := []int64{4021, 1187, 9903}

	pharmacy := pipeline.DebugDecisionKey(productmatch.NormalizeText(raw), candidates)
	vendor := ingest.DebugDecisionKey(productmatch.NormalizeText(raw), candidates)

	if pharmacy != vendor {
		t.Fatalf("the same question hashes differently:\n  smart order: %s\n  vendor:      %s",
			pharmacy, vendor)
	}
	if pharmacy == "" {
		t.Fatal("the key is empty")
	}
}

// TestCacheKeyIgnoresCandidateOrder: two importers retrieving the same products
// in a different order asked the same question, and a cache that misses on the
// ordering is a cache that misses.
func TestCacheKeyIgnoresCandidateOrder(t *testing.T) {
	a := matchflow.DecisionKey("panadol extra", []int64{3, 1, 2})
	b := matchflow.DecisionKey("panadol extra", []int64{1, 2, 3})
	if a != b {
		t.Errorf("candidate order changed the key")
	}
	if c := matchflow.DecisionKey("panadol extra", []int64{1, 2, 2, 3}); c != b {
		t.Errorf("a repeated candidate changed the key")
	}
}

// TestCacheKeySeparatesDifferentQuestions is the other half. A shortlist is part
// of the question — the model may only answer from what it was shown — so the
// same text against different candidates must not reuse an answer.
func TestCacheKeySeparatesDifferentQuestions(t *testing.T) {
	base := matchflow.DecisionKey("augmentin 1g", []int64{1, 2})

	if same := matchflow.DecisionKey("augmentin 625", []int64{1, 2}); same == base {
		t.Error("different text produced the same key")
	}
	if same := matchflow.DecisionKey("augmentin 1g", []int64{1, 2, 3}); same == base {
		t.Error("a different shortlist produced the same key")
	}
	// A name that contains the delimiter must not be able to forge a key: if
	// the separator appeared in the data, "a\x1f1," and "a" with candidate 1
	// would collide.
	if forged := matchflow.DecisionKey("augmentin 1g\x1f1,", nil); forged == base {
		t.Error("a crafted name collided with a real key")
	}
}

// TestSmartOrderAndVendorShareTheEnhancerContract checks that the two modules
// still speak the same types, which is what lets one gateway adapter serve both.
func TestSmartOrderAndVendorShareTheEnhancerContract(t *testing.T) {
	var batch matchflow.Batch
	// These assignments only compile while all three names refer to one type.
	var pipelineBatch pipeline.EnhanceBatch = batch
	var ingestBatch ingest.EnhanceBatch = batch
	_ = pipelineBatch
	_ = ingestBatch

	var decision matchflow.Decision
	var pipelineOut pipeline.EnhanceOutcome = decision
	var ingestOut ingest.EnhanceOutcome = decision
	_ = pipelineOut
	_ = ingestOut

	// And the ceilings come from one table, so a change to either profile is a
	// change to a documented argument rather than to a number in a const block.
	if matchflow.For(matchflow.ProfileOrder).MinApplyConfidence <= 0 {
		t.Error("the order profile has no apply floor")
	}
	if matchflow.For(matchflow.ProfileVendor).MinPlausible <= 0 {
		t.Error("the vendor profile has no plausibility floor")
	}
}

// TestSmartOrderLineNormalisationMatchesTheKey guards the last link: the key is
// built from a normalised name, and both importers must normalise the same way.
func TestSmartOrderLineNormalisationMatchesTheKey(t *testing.T) {
	line := &smartorder.Line{RawName: "أوجمنتين 1 جم 14 قرص"}
	pipeline.Normalize([]*smartorder.Line{line})

	if line.NormName != productmatch.NormalizeText(line.RawName) {
		t.Fatalf("the smart order normalises differently from the shared function:\n  %q\n  %q",
			line.NormName, productmatch.NormalizeText(line.RawName))
	}
}
