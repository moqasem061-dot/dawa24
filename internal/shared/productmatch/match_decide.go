package productmatch

// Turning ranked candidates into an answer.
//
// match.go retrieves and ranks. This file decides — including the decision that
// matters most and is easiest to get wrong, which is the refusal to decide at
// all when two catalogue products fit a row equally well.

import "fmt"

// tieMargin is how close two candidates' scores have to be before the decision
// between them is treated as one arithmetic alone should not make.
//
// It was three hundredths, and it was also disabled above 0.97 — the exact
// band, where two entries of one brand family land most often, and where the
// old score clamp put every near-identical pair at the identical value. So the
// one situation the check existed for was the one situation it sat out.
const tieMargin = 0.06

// decide turns the ranked candidates into an answer, including the answer
// "these two fit equally well and I will not choose between them".
//
// Closeness alone no longer decides that. Two candidates are ambiguous when
// they are close AND nothing the row states picks one of them — see
// Index.separated. That distinction is what lets the engine settle "اماريل ام
// 2/500" against a family of four confidently, while refusing to settle a row
// that named a brand and no attribute at all against the same four.
func (idx *Index) decide(q *query, scored []scoredProduct, opts MatchOptions) MatchResult {
	best := scored[0]
	res := MatchResult{
		ProductID:  best.product.ID,
		Score:      best.score,
		Candidates: describe(scored, opts.MaxCandidates),
	}

	tied := len(scored) > 1 &&
		best.score-scored[1].score < tieMargin &&
		!idx.separated(q, best.product, scored[1].product)

	switch {
	case best.score < opts.MinReview:
		// Everything found contradicts the row badly enough that none of it can
		// be offered as an answer. The shortlist still travels with the result,
		// because "nothing matched" and "these three were close and every one
		// of them disagrees about the dose" are different things to be told.
		res.ProductID = 0
		res.Level = MatchNone
		res.Reason = "أقرب الأصناف في الكتالوج تختلف في خصائص أساسية عن هذا الصف: " +
			best.describeReason()
	case tied:
		res.Level = MatchAmbiguous
		res.Reason = fmt.Sprintf(
			"صنفان في الكتالوج بنفس درجة التطابق (%d%%) ولا يوجد في الصف ما يفرّق بينهما؛ "+
				"يلزم اختيار الصحيح يدوياً.",
			int(best.score*100))
	case !best.settleable():
		// The letters line up and no word does. Offered, never applied — see
		// scoredProduct.settleable.
		res.Level = MatchReview
		res.Reason = fmt.Sprintf(
			"%s (%d%%) — التشابه في حروف الاسم فقط، ولا تتطابق أي كلمة مميزة؛ يلزم التأكيد",
			best.describeReason(), int(best.score*100))
	case best.exact:
		res.Level = MatchExact
		res.Reason = "تطابق تام لاسم الصنف بعد المعايرة مع توافق الخصائص"
	case best.score >= opts.MinStrong:
		res.Level = MatchStrong
		res.Reason = fmt.Sprintf("%s (%d%%)", best.describeReason(), int(best.score*100))
	default:
		res.Level = MatchReview
		res.Reason = fmt.Sprintf("%s (%d%%) — يحتاج تأكيداً",
			best.describeReason(), int(best.score*100))
	}
	return res
}

// describe renders the shortlist for the review screen.
func describe(scored []scoredProduct, limit int) []MatchCandidate {
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]MatchCandidate, 0, len(scored))
	for _, s := range scored {
		name := s.product.NameAR
		if name == "" {
			name = s.product.NameEN
		}
		out = append(out, MatchCandidate{
			ProductID:     s.product.ID,
			Name:          name,
			Scientific:    s.product.Scientific,
			DosageForm:    s.product.DosageForm,
			Concentration: s.product.Concentration,
			Manufacturer:  s.product.Manufacturer,
			PublicPrice:   s.product.PublicPrice,
			Score:         s.score,
			Reason:        s.describeReason(),
		})
	}
	return out
}
