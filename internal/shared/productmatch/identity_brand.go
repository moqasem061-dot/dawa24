package productmatch

import (
	"regexp"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

var alternativeRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:,\s*|\s+)(?:alternative\s+to|substitute\s+for|alt\s+to)\s+.*$`),
	regexp.MustCompile(`(?i)(?:,\s*|\s+)(?:بديل\s+لـ?|بديل)\s+.*$`),
	regexp.MustCompile(`(?i)\((?:alternative\s+to|substitute\s+for|alt\s+to|بديل\s+لـ?|بديل)\s+[^)]+\)`),
}

// StripAlternativeClause removes trailing or parenthetical alternative/substitution phrases.
// e.g. "Brand A 0.5g, alternative to Brand B" -> "Brand A 0.5g"
func StripAlternativeClause(name string) string {
	s := name
	for _, re := range alternativeRegexes {
		s = re.ReplaceAllString(s, "")
	}
	return strings.TrimSpace(strings.TrimRight(s, ",;،؛ "))
}

// compareBrand compares brand tokens between row and product facts.
// When the row and the candidate each carry a distinct identifiable brand token
// and they differ, that is a brand hard conflict.
func compareBrand(row, prod IdentityFacts, comp *Comparison) {
	if len(row.Brand) == 0 || len(prod.Brand) == 0 {
		comp.Attributes["brand"] = OutcomeUnstated
		return
	}

	rowTokens := filterBrandTokens(row.Brand)
	prodTokens := filterBrandTokens(prod.Brand)

	if len(rowTokens) == 0 || len(prodTokens) == 0 {
		comp.Attributes["brand"] = OutcomeUnstated
		return
	}

	// Exact / normalized match of full brand strings
	joinedRow := strings.Join(rowTokens, " ")
	joinedProd := strings.Join(prodTokens, " ")
	if brandsMatch(joinedRow, joinedProd) {
		comp.Attributes["brand"] = OutcomeAgree
		return
	}

	// Single-token brand comparison
	if len(rowTokens) == 1 && len(prodTokens) == 1 {
		if brandsMatch(rowTokens[0], prodTokens[0]) {
			comp.Attributes["brand"] = OutcomeAgree
		} else {
			comp.Attributes["brand"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictBrand)
		}
		return
	}

	// Distinct identifiable brand tokens comparison.
	// Primary brand token is the leading distinctive non-maker token.
	rowPrimary := leadingBrandToken(rowTokens)
	prodPrimary := leadingBrandToken(prodTokens)

	if rowPrimary != "" && prodPrimary != "" {
		if !brandsMatch(rowPrimary, prodPrimary) {
			// Distinct brand tokens differ -> hard conflict
			comp.Attributes["brand"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictBrand)
			return
		}
	}

	// Check if all tokens of either side match
	matches := 0
	for _, rt := range rowTokens {
		for _, pt := range prodTokens {
			if brandsMatch(rt, pt) {
				matches++
				break
			}
		}
	}

	if matches == len(rowTokens) && matches == len(prodTokens) {
		comp.Attributes["brand"] = OutcomeAgree
	} else if matches == len(rowTokens) || matches == len(prodTokens) {
		// Subset match with no conflicting primary token
		comp.Attributes["brand"] = OutcomeAgree
	} else {
		comp.Attributes["brand"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictBrand)
	}
}

func leadingBrandToken(tokens []string) string {
	for _, t := range tokens {
		if t == "brand" || t == "product" || t == "براند" || t == "منتج" {
			// Generic brand label prefix, check next token (e.g. "Brand A" -> "a")
			continue
		}
		return t
	}
	if len(tokens) > 0 {
		return tokens[0]
	}
	return ""
}

func filterBrandTokens(tokens []Token) []string {
	var out []string
	for _, t := range tokens {
		txt := strings.ToLower(strings.TrimSpace(t.Text))
		if txt == "" || hasDigit(txt) || isNumericLiteral(txt) {
			continue
		}
		key := sheet.NormalizeKey(txt)
		if _, ok := unitWords[key]; ok {
			continue
		}
		if _, ok := formWords[key]; ok {
			continue
		}
		if noiseWords[txt] || noiseWords[key] || isNoiseOrMakerWord(txt) {
			continue
		}
		out = append(out, txt)
	}
	return out
}

func isNoiseOrMakerWord(w string) bool {
	noise := map[string]bool{
		"شركة": true, "مستحضر": true, "دواء": true, "علاج": true,
		"co": true, "pharma": true, "lab": true, "inc": true,
		"باكت": true, "باكيت": true, "علبة": true, "علبه": true,
		"شريط": true, "كرتونة": true, "كرتونه": true, "pack": true,
		"strip": true, "box": true,
	}
	return noise[w]
}

func brandsMatch(a, b string) bool {
	if a == b {
		return true
	}
	normA := foldAlef(a)
	normB := foldAlef(b)
	if normA == normB {
		return true
	}
	skA := Skeleton(a)
	skB := Skeleton(b)
	if skA != "" && skB != "" && skA == skB {
		return true
	}
	return false
}
