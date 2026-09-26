package aicapabilities

// The pair check: one line, one catalogue product, same or not?
//
// See internal/shared/matchflow/check.go for why it exists. This file is the
// prompt, the schema and the call. It rides the same capability as the
// proposal pass — same model tier, same budget, same usage ledger — and
// differs only in the question.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

const verifySystemPrompt = `You verify product matches for Dawa24, an Egyptian pharmacy marketplace. A line typed by a pharmacy or supplier (usually Arabic, often in Egyptian pharmacy shorthand) has been proposed as a match for ONE catalogue product. You decide, for each pair, whether the line and the product are the SAME sellable product.

Nobody else will check your answer. If you say "same", a pharmacy orders this exact product. A wrong "same" delivers the wrong medicine; a wrong "different" costs one minute of a person's time. Be generous about SPELLING and strict about IDENTITY.

# INPUT

One row per pair:
  ref|line_text|line_brand_hint|line_strength_hint|line_form_hint|line_pack_hint || product_id|arabic_name|english_name|scientific_name|dosage_form|strength|manufacturer
line_text is exactly what was typed. The *_hint fields come from an automatic parser and are sometimes wrong — trust line_text over them.

# JUDGE FOUR ATTRIBUTES, THEN THE VERDICT

brand — is it the same brand / product name?
  "same": the same name in any spelling. Arabic transliterations of one Latin brand are the same: ابليفاى = ابيليفاي = abilify; بنادول = بانادول; ى/ي, ة/ه, أ/إ/آ/ا interchange freely; long vowels come and go; one word or two (ارمو ويك = ارموويك). Use english_name to confirm a transliteration.
  "different": a different brand, EVEN IF it is the same molecule. Two brands of one molecule are two products (اريكتامكس is not سياليس). Sharing a prefix is not sharing a brand (امبريد ≠ امبريديل). Sharing a manufacturer or distributor is never evidence.
  "unclear": you cannot tell.

strength — do the strengths agree?
  "same": every strength stated on both sides is equal (500 mg = 500مجم = 0.5 g).
  "different": any stated number disagrees: 5 mg vs 10 mg, 0.025% vs 0.05%, 32/25 vs 16/12.5. Never round.
  "unstated": one side gives no strength and nothing contradicts. This is common and not a problem.
  Numbers attached to ج, or after س.ج / سعر, are PRICES in pounds, not strengths. Pack counts (20 قرص, 3 شرائط) are not strengths.

form — same dosage form and route?
  "same" or "unstated" as above. Tablet written as capsule or the reverse is "same" (pharmacies use them loosely for solid oral forms).
  "different": syrup vs tablet, injection vs oral, eye drops vs eye gel, cream vs oral, sachet vs tablet, orally-disintegrating (ODT, ديسكملت, زيديس, ذائبة بالفم) vs ordinary tablet.

variant — same line extension and same ingredient combination?
  "same": neither side carries an extension word the other lacks, and both are the same single ingredient or the same combination.
  "different": one side has an identity word the other does not — plus/بلس, forte/فورت, extra/اكسترا, advance/ادفانس, night/نايت, day/داي, cold/كولد, flu/فلو, fast/فاست, max/ماكس, ultra/الترا, total/توتال, gold/جولد, lite/لايت, zero/زيرو, baby/بيبي, junior/جونيور, duo/دوو, retard/ريتارد, or a release code SR CR XR MR XL LA — or one is a combination and the other a single ingredient (اتاكاند ≠ اتاكاند بلس; اموكسيسيللين ≠ اوجمنتين).

Ignore in line_text: price notes (سعر جديد, س.ج 137ج, عرض, خصم), the distributor after "/", and the pharmacist's descriptive notes (مسكن, للبرد, للاطفال). Pack size is not identity.

verdict:
  "same" ONLY IF brand is "same", variant is "same", and neither strength nor form is "different".
  "different" if any attribute is "different".
  "unsure" otherwise.

confidence: how sure you are of the verdict, 0.0–1.0. A "same" below 0.80 is treated as unsure.

# OUTPUT

ONE JSON object, nothing else:
{"results":[{"ref":<int>,"brand":"same|different|unclear","strength":"same|different|unstated|unclear","form":"same|different|unstated|unclear","variant":"same|different","verdict":"same|different|unsure","confidence":<0-1>}]}
Write the fields in exactly that order. One result per ref, every ref. Add "reason" (a few words) only when the verdict is "different" or "unsure".`

func verifySchema() map[string]any {
	enum := func(vals ...string) map[string]any {
		return map[string]any{"type": "string", "enum": vals}
	}
	return map[string]any{
		"name":   "dawa24_match_verification",
		"strict": true,
		"schema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"results"},
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required": []string{"ref", "brand", "strength", "form", "variant",
							"verdict", "confidence"},
						"properties": map[string]any{
							"ref":        map[string]any{"type": "integer"},
							"brand":      enum("same", "different", "unclear"),
							"strength":   enum("same", "different", "unstated", "unclear"),
							"form":       enum("same", "different", "unstated", "unclear"),
							"variant":    enum("same", "different"),
							"verdict":    enum("same", "different", "unsure"),
							"confidence": map[string]any{"type": "number"},
							"reason":     map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}

// RenderVerifyInput builds the user message for a pair check. Pure and
// deterministic, like RenderEnhanceInput.
func RenderVerifyInput(batch matchflow.CheckBatch) string {
	var b strings.Builder
	b.Grow(len(batch.Pairs)*256 + 128)
	b.WriteString("PAIRS\n")
	b.WriteString("# ref|line_text|line_brand_hint|line_strength_hint|line_form_hint|line_pack_hint || product_id|arabic_name|english_name|scientific_name|dosage_form|strength|manufacturer\n")
	for _, p := range batch.Pairs {
		b.WriteString(strconv.Itoa(p.Ref))
		writeField(&b, p.Item.Text)
		writeField(&b, p.Item.Brand)
		writeField(&b, p.Item.Strength)
		writeField(&b, p.Item.DosageForm)
		writeField(&b, packField(p.Item.PackSize))
		b.WriteString(" || ")
		b.WriteString(strconv.FormatInt(p.Product.ProductID, 10))
		writeField(&b, p.Product.NameAR)
		writeField(&b, p.Product.NameEN)
		writeField(&b, p.Product.Scientific)
		writeField(&b, p.Product.DosageForm)
		writeField(&b, p.Product.Concentration)
		writeField(&b, p.Product.Manufacturer)
		b.WriteByte('\n')
	}
	return b.String()
}

// DecodeVerifyResponse parses a pair-check response. Tolerant of packaging,
// strict about content — exactly as DecodeEnhanceResponse.
func DecodeVerifyResponse(content string) ([]matchflow.Check, error) {
	clean := extractJSONObject(content)
	if clean == "" {
		return nil, fmt.Errorf("aicapabilities: no JSON object in verification response")
	}
	var wrapper struct {
		Results []matchflow.Check `json:"results"`
	}
	if err := json.Unmarshal([]byte(clean), &wrapper); err != nil {
		return nil, fmt.Errorf("aicapabilities: decode verification: %w", err)
	}
	return wrapper.Results, nil
}

// Verify satisfies matchflow.Verifier.
func (s *Service) Verify(ctx context.Context, batch matchflow.CheckBatch) ([]matchflow.Check, error) {
	if len(batch.Pairs) == 0 {
		return nil, nil
	}
	if s.gw == nil {
		return nil, fmt.Errorf("aicapabilities: gateway not configured")
	}
	orgID, userID, vKey := s.attribution(ctx, batch.OrganizationID, batch.UserID)

	resp, err := s.gw.Invoke(ctx, gateway.Request{
		Capability:     gateway.CapMatchEnhance,
		System:         verifySystemPrompt,
		Input:          RenderVerifyInput(batch),
		Schema:         verifySchema(),
		OrganizationID: orgID,
		UserID:         userID,
		VirtualKey:     vKey,
		Feature:        batch.Feature,
		MaxTokens:      verifyMaxTokens(len(batch.Pairs)),
	})
	if err != nil {
		s.log.WarnContext(ctx, "match verification failed; proposals stay unapplied",
			"pairs", len(batch.Pairs), "err", err)
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("aicapabilities: empty verification response")
	}
	checks, err := DecodeVerifyResponse(resp.Content)
	if err != nil {
		s.log.WarnContext(ctx, "could not read verification response",
			"pairs", len(batch.Pairs), "err", err)
		return nil, err
	}
	s.log.InfoContext(ctx, "match verification resolved",
		"pairs", len(batch.Pairs), "checks", len(checks),
		"req_id", resp.RequestID, "model", resp.Model,
		"in_tok", resp.InputTok, "out_tok", resp.OutputTok)
	return checks, nil
}

// Enhance satisfies matchflow.Enhancer, so the service is handed to every
// importer as-is and the pair check is discovered on the same value.
func (s *Service) Enhance(ctx context.Context, batch matchflow.Batch) ([]matchflow.Decision, error) {
	return s.EnhanceMatches(ctx, batch)
}

// verifyMaxTokens sizes a pair check's answer: roughly seventy tokens of JSON
// per pair plus headroom for a model that reasons before it writes.
func verifyMaxTokens(pairs int) int {
	return min(pairs*110+2048, 32000)
}
