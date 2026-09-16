package aicapabilities

import (
	"strings"
	"testing"
)

// The prompt generator is a pure function, and the decision cache depends on it
// being one. These tests hold that line.

func sampleRequest() EnhanceRequest {
	id := int64(101)
	return EnhanceRequest{
		Catalog: []CatalogEntry{
			{ProductID: 101, NameAR: "ابيليفاي 10مجم 10 اقراص", NameEN: "abilify 10 mg 10 tabs",
				DosageForm: "tablet", Concentration: "10 mg", Manufacturer: "otsuka"},
			{ProductID: 102, NameAR: "ارموويك 50مجم 10 اقراص"},
		},
		Items: []EnhanceItem{
			{Ref: 1, Text: "ابليفاى 10مجم 10قرص", Brand: "ابليفاى", Strength: "10 مجم",
				DosageForm: "أقراص", PackSize: 10, CurrentGuess: &id, CurrentScore: 0.42,
				Options: []int64{101, 102}},
		},
	}
}

// The same request must render byte-identically every time. A prompt that
// varies is a cache that never hits and a bill that never stops.
func TestRenderIsDeterministic(t *testing.T) {
	req := sampleRequest()
	first := RenderEnhanceInput(req)
	for i := 0; i < 20; i++ {
		if got := RenderEnhanceInput(req); got != first {
			t.Fatalf("render %d differed from the first", i)
		}
	}
}

func TestRenderCarriesBothSectionsAndTheColumnKeys(t *testing.T) {
	out := RenderEnhanceInput(sampleRequest())

	for _, want := range []string{
		"CATALOG\n", "\nITEMS\n",
		"101|ابيليفاي 10مجم 10 اقراص|abilify 10 mg 10 tabs||tablet|10 mg|otsuka",
		"1|new|ابليفاى 10مجم 10قرص|ابليفاى|10 مجم|أقراص|10||101@0.42|101,102",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered input is missing %q\n---\n%s", want, out)
		}
	}
}

// A product with no guess renders "-" rather than an empty cell, so the model
// can tell "the engine had no opinion" from "the engine's opinion is missing".
func TestRenderMarksAnAbsentGuess(t *testing.T) {
	req := sampleRequest()
	req.Items[0].CurrentGuess = nil
	if !strings.Contains(RenderEnhanceInput(req), "|-|101,102") {
		t.Errorf("an absent guess did not render as \"-\":\n%s", RenderEnhanceInput(req))
	}
}

// 🚦 A pipe inside an Egyptian product name would silently shift every later
// column of that row, and the model would read the manufacturer as the dose.
func TestRenderStripsTheDelimiterFromValues(t *testing.T) {
	req := sampleRequest()
	req.Catalog[1].NameAR = "منتج | به فاصل\nوسطر جديد"

	out := RenderEnhanceInput(req)
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "102|") {
			continue
		}
		if n := strings.Count(line, "|"); n != 6 {
			t.Fatalf("row has %d delimiters, want 6: %q", n, line)
		}
		return
	}
	t.Fatal("the catalogue row was not rendered at all")
}

// The response contract has to survive the packaging models add around JSON.
func TestDecodeToleratesPackagingAroundTheJSON(t *testing.T) {
	cases := map[string]string{
		"bare":            `{"results":[{"ref":1,"product_id":101,"confidence":0.9,"reason":"نفس التركيز"}]}`,
		"fenced":          "```json\n{\"results\":[{\"ref\":1,\"product_id\":101,\"confidence\":0.9}]}\n```",
		"prose before":    "Here is the answer:\n{\"results\":[{\"ref\":1,\"product_id\":101,\"confidence\":0.9}]}",
		"note after":      `{"results":[{"ref":1,"product_id":101,"confidence":0.9}]} — hope this helps`,
		"brace in reason": `{"results":[{"ref":1,"product_id":101,"confidence":0.9,"reason":"تطابق {مؤكد}"}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeEnhanceResponse(content)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != 1 || got[0].Ref != 1 ||
				got[0].ProductID == nil || *got[0].ProductID != 101 {
				t.Fatalf("decoded %+v", got)
			}
		})
	}
}

// A null product id is a real answer — "none of these" — and must survive
// decoding as one rather than as a zero.
func TestDecodeKeepsAnAbstentionDistinctFromZero(t *testing.T) {
	got, err := DecodeEnhanceResponse(`{"results":[{"ref":3,"product_id":null,"confidence":0.4}]}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ProductID != nil {
		t.Fatalf("an abstention did not decode as nil: %+v", got)
	}
}

// Answers are sorted by ref, so application order does not depend on the order
// the model happened to reply in.
func TestDecodeSortsByRef(t *testing.T) {
	got, err := DecodeEnhanceResponse(
		`{"results":[{"ref":7,"product_id":null,"confidence":0},{"ref":2,"product_id":null,"confidence":0}]}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Ref != 2 || got[1].Ref != 7 {
		t.Fatalf("results are not ordered by ref: %+v", got)
	}
}

func TestDecodeRejectsAResponseWithNoJSON(t *testing.T) {
	if _, err := DecodeEnhanceResponse("I could not match any of these items."); err == nil {
		t.Fatal("prose was accepted as a decision set")
	}
}

// The system prompt states the rules the applier also enforces. If the two ever
// disagree the model is being asked for answers the pipeline will throw away,
// so the overlap is asserted rather than assumed.
func TestSystemPromptStatesTheRulesTheApplierEnforces(t *testing.T) {
	for _, want := range []string{
		"DIFFERENT STRENGTH",
		"DIFFERENT LINE EXTENSION",
		"DIFFERENT BRAND, SAME MOLECULE",
		"SHARED COMPANY IS NOT SHARED IDENTITY",
		"MUST NOT output an id that does not appear in the CATALOG section",
		"below 0.80",
		`{"results":[`,
	} {
		if !strings.Contains(enhanceSystemPrompt, want) {
			t.Errorf("system prompt no longer states %q", want)
		}
	}
}
