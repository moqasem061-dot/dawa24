package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestQuestionSignature_DifferentiatesStrengths(t *testing.T) {
	normName := "concor"

	// Same raw name, different structured strength: 10 mg vs 20 mg
	facts10 := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "concor"}},
		Strengths: []productmatch.Strength{
			{Value: "10", Unit: "mg"},
		},
	}
	facts20 := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "concor"}},
		Strengths: []productmatch.Strength{
			{Value: "20", Unit: "mg"},
		},
	}

	sig10 := productmatch.QuestionSignature(facts10, normName, productmatch.NormalizerVersion)
	sig20 := productmatch.QuestionSignature(facts20, normName, productmatch.NormalizerVersion)

	if sig10 == sig20 {
		t.Fatalf("expected different signatures for 10mg vs 20mg, got identical %q", sig10)
	}

	// Determinism: same input produces identical signature
	sig10Repeat := productmatch.QuestionSignature(facts10, normName, productmatch.NormalizerVersion)
	if sig10 != sig10Repeat {
		t.Fatalf("signature is not deterministic: %q != %q", sig10, sig10Repeat)
	}

	// Normalizer version sensitivity
	sigV2 := productmatch.QuestionSignature(facts10, normName, "n2")
	if sig10 == sigV2 {
		t.Fatalf("expected different signature for different normalizer versions")
	}
}

func TestQuestionSignature_IdentifiersDifferentiate(t *testing.T) {
	normName := "panadol"
	facts1 := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "panadol"}},
		Identifiers: []productmatch.Identifier{
			{Namespace: "gtin", Value: "6223000550014"},
		},
	}
	facts2 := productmatch.IdentityFacts{
		Brand: []productmatch.Token{{Text: "panadol"}},
		Identifiers: []productmatch.Identifier{
			{Namespace: "gtin", Value: "6223000550021"},
		},
	}

	sig1 := productmatch.QuestionSignature(facts1, normName, productmatch.NormalizerVersion)
	sig2 := productmatch.QuestionSignature(facts2, normName, productmatch.NormalizerVersion)

	if sig1 == sig2 {
		t.Fatalf("expected different signatures for different identifiers")
	}
}
