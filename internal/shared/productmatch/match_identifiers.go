package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// matchByBarcode resolves the one identifier that means the same package.
func (idx *Index) matchByBarcode(row *Row) (MatchResult, bool) {
	code := sheet.DigitsOnly(row.Barcode)
	if code == "" || len(code) < 8 {
		return MatchResult{}, false
	}
	hits := idx.byBarcode[code]
	switch len(hits) {
	case 0:
		return MatchResult{}, false
	case 1:
		return MatchResult{
			ProductID: hits[0].ID, Level: MatchBarcode, Score: 1,
			Reason: "مطابقة تامة عبر الباركود الدولي",
		}, true
	}
	// Several catalogue products share the barcode, which happens for a
	// multi-pack registered under one GTIN. Fall through to scoring so the name
	// and the dose can separate them, but keep the shortlist.
	return MatchResult{}, false
}

// matchByCode resolves a supplier code the catalogue also carries.
//
// It demands a corroborating name similarity by default, because a bare numeric
// code colliding across two numbering schemes is common and the consequence —
// the wrong medicine priced — is not recoverable by looking at the result.
// CodeIsAuthoritative lifts that where the user has stated the column holds the
// catalogue's own code.
func (idx *Index) matchByCode(row *Row, q *query, opts MatchOptions) (MatchResult, bool) {
	key := sheet.NormalizeCode(row.SKU)
	if len([]rune(key)) < 4 {
		return MatchResult{}, false
	}
	hits := idx.bySKU[key]
	if len(hits) != 1 {
		return MatchResult{}, false
	}
	p := hits[0]
	if !opts.CodeIsAuthoritative && len(q.tokens) > 0 && idx.nameSimilarity(q, p) < 0.35 {
		return MatchResult{}, false
	}
	return MatchResult{
		ProductID: p.ID, Level: MatchCode, Score: 0.95,
		Reason: "مطابقة عبر كود الصنف مع تشابه في الاسم",
	}, true
}
