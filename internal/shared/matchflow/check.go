package matchflow

// The second question: is THIS line THIS product?
//
// The proposal pass asks the model to choose from a list, and a list is an
// invitation. Shown sixteen candidates, a model that knows the right answer is
// absent will still name the one that looks closest far more often than it
// answers null — the options are on the table, the task says "choose", and
// "بانادول اكسترا" does look very like "بانادول". That selection bias is the
// forced match, and no amount of prompt wording removes it, because the
// wording is not what causes it: the list is.
//
// So an answer that would CHANGE a row is not taken on the proposal's word. It
// is put back to the model as a different question, with the list gone and
// the engine's guess gone: one line, one catalogue product, same or not? A
// yes/no judgement about a single pair has nothing to anchor on and nothing to
// settle for, and the model has to walk the identity attributes one by one
// before it is allowed to say "same". Only when both independent questions
// agree is a match applied.
//
// The attributes are asked for BEFORE the verdict, in the schema's own field
// order, because a model writes its answer left to right: a verdict written
// after "strength: different" is a verdict that has already read it.

import (
	"context"
	"strings"
)

// Pair is one line and one catalogue product.
type Pair struct {
	// Ref is request-local, as it is for Item.
	Ref     int
	Item    Item
	Product CatalogEntry
}

// CheckBatch is one verification request.
type CheckBatch struct {
	Pairs []Pair

	// Attribution, exactly as on Batch.
	Feature        string
	OrganizationID int64
	UserID         int64
}

// Attribute judgements. "unstated" means one side does not say, which is not a
// contradiction — the catalogue routinely spells out what a pharmacy left off.
const (
	AttrSame      = "same"
	AttrDifferent = "different"
	AttrUnstated  = "unstated"
	AttrUnclear   = "unclear"
)

// Check verdicts.
const (
	CheckSame      = "same"
	CheckDifferent = "different"
	CheckUnsure    = "unsure"
)

// Check is the model's judgement about one pair.
type Check struct {
	Ref int `json:"ref"`
	// Brand: is it the same brand or product line, spelling aside?
	Brand string `json:"brand"`
	// Strength: do the stated strengths agree, number for number?
	Strength string `json:"strength"`
	// Form: same dosage form and route?
	Form string `json:"form"`
	// Variant: the same line extension and ingredient combination — plus,
	// forte, extra, SR, a second active ingredient?
	Variant    string  `json:"variant"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// Accepts reports whether a check establishes identity.
//
// Every attribute is gated, not only the verdict. A model that wrote
// "strength: different" and then "verdict: same" has contradicted itself, and a
// self-contradicting answer about a medicine is not an answer.
func (c Check) Accepts(floor float64) bool {
	if c.Verdict != CheckSame || c.Confidence < floor || c.Confidence > 1 {
		return false
	}
	if c.Brand != AttrSame || c.Variant != AttrSame {
		return false
	}
	return c.Strength != AttrDifferent && c.Form != AttrDifferent &&
		c.Strength != AttrUnclear && c.Form != AttrUnclear
}

// Rejects reports whether a check positively establishes that the pair is two
// different products, as opposed to merely failing to establish identity.
func (c Check) Rejects(floor float64) bool {
	if c.Confidence < floor || c.Confidence > 1 {
		return false
	}
	return c.Verdict == CheckDifferent || c.Brand == AttrDifferent ||
		c.Variant == AttrDifferent || c.Strength == AttrDifferent || c.Form == AttrDifferent
}

// Verifier answers pair questions. The production capability implements it
// alongside Enhancer, and Run discovers it by assertion; an Enhancer without
// it (a test double) runs the proposal pass alone.
type Verifier interface {
	Verify(ctx context.Context, batch CheckBatch) ([]Check, error)
}

// normalizeCheck folds case and whitespace, so "Same " and "same" are one
// answer and an unrecognised value fails every gate rather than slipping one.
func normalizeCheck(c Check) Check {
	c.Brand = normAttr(c.Brand)
	c.Strength = normAttr(c.Strength)
	c.Form = normAttr(c.Form)
	c.Variant = normAttr(c.Variant)
	c.Verdict = normAttr(c.Verdict)
	return c
}

func normAttr(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Verified reports whether an answer names a product that survived the pair
// check.
func (a Answer) Verified() bool {
	return a.Checked && a.Decision.ProductID != nil
}
