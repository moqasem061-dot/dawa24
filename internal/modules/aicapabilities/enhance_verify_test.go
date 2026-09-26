package aicapabilities

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// The pair check must not see the engine's guess or the shortlist; the
// renderer is where that is guaranteed on the wire.
func TestVerifyInputCarriesNoAnchors(t *testing.T) {
	guess := int64(99)
	in := RenderVerifyInput(matchflow.CheckBatch{Pairs: []matchflow.Pair{{
		Ref:     1,
		Item:    matchflow.Item{Text: "بانادول|اكسترا", Strength: "500مجم", CurrentGuess: &guess, Options: []int64{5, 6}},
		Product: matchflow.CatalogEntry{ProductID: 45, NameAR: "بانادول اكسترا", NameEN: "Panadol Extra"},
	}}})
	if strings.Contains(in, "99") || strings.Contains(in, "5,6") {
		t.Errorf("pair input leaked an anchor:\n%s", in)
	}
	if !strings.Contains(in, "1|بانادول اكسترا|") {
		t.Errorf("a pipe inside the text was not neutralised:\n%s", in)
	}
	if !strings.Contains(in, "|| 45|بانادول اكسترا|Panadol Extra") {
		t.Errorf("product side missing:\n%s", in)
	}
}

// Rendering is a pure function: identical input, identical bytes.
func TestVerifyInputIsDeterministic(t *testing.T) {
	b := matchflow.CheckBatch{Pairs: []matchflow.Pair{
		{Ref: 1, Item: matchflow.Item{Text: "a"}, Product: matchflow.CatalogEntry{ProductID: 1}},
		{Ref: 2, Item: matchflow.Item{Text: "b"}, Product: matchflow.CatalogEntry{ProductID: 2}},
	}}
	if RenderVerifyInput(b) != RenderVerifyInput(b) {
		t.Fatal("render is not deterministic")
	}
}

func TestDecodeVerifyResponseToleratesPackaging(t *testing.T) {
	got, err := DecodeVerifyResponse("Here you go:\n```json\n" +
		`{"results":[{"ref":1,"brand":"same","strength":"different","form":"same","variant":"same","verdict":"different","confidence":0.97,"reason":"500 vs 1000 {mg}"}]}` +
		"\n```")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Strength != "different" || got[0].Reason != "500 vs 1000 {mg}" {
		t.Fatalf("decoded %+v", got)
	}
}

func TestDecodeVerifyResponseRejectsNonJSON(t *testing.T) {
	if _, err := DecodeVerifyResponse("I cannot help with that."); err == nil {
		t.Fatal("expected an error for a response with no JSON")
	}
}

// The completion budget must cover a JSON answer per item plus reasoning
// headroom; twenty-eight tokens an item truncated real batches.
func TestCompletionBudgetsCoverTheAnswer(t *testing.T) {
	if n := enhanceMaxTokens(60); n < 60*80 {
		t.Errorf("proposal budget %d for 60 items is too small", n)
	}
	if n := verifyMaxTokens(40); n < 40*80 {
		t.Errorf("check budget %d for 40 pairs is too small", n)
	}
}
