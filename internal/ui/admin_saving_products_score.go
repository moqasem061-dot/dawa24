package ui

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// calculateSavingMatchScore returns a similarity score [0.30, 1.00] between a raw uploaded name and a candidate product.
func calculateSavingMatchScore(raw string, p *catalog.Product) float64 {
	rawClean := cleanScoreString(raw)
	if rawClean == "" {
		return 0.50
	}

	nameARClean := cleanScoreString(p.Name.Get(i18n.AR))
	nameENClean := cleanScoreString(p.Name.Get(i18n.EN))
	skuClean := cleanScoreString(p.SKU)
	sciClean := cleanScoreString(p.ScientificName)

	if rawClean == nameARClean || rawClean == nameENClean || (skuClean != "" && rawClean == skuClean) {
		return 1.00
	}

	scoreAR := stringTokenSimilarity(rawClean, nameARClean)
	scoreEN := stringTokenSimilarity(rawClean, nameENClean)
	scoreSci := stringTokenSimilarity(rawClean, sciClean) * 0.90

	best := max(scoreAR, scoreEN)
	best = max(best, scoreSci)

	if best < 0.30 {
		best = 0.30
	}
	if best > 0.99 {
		best = 0.99
	}
	return best
}

func cleanScoreString(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(
		"أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا",
		"ة", "ه", "ى", "ي",
		"-", " ", "_", " ", "/", " ", ",", " ",
	)
	return replacer.Replace(s)
}

func stringTokenSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if strings.HasPrefix(b, a) || strings.HasPrefix(a, b) {
		return 0.92
	}
	if strings.Contains(b, a) || strings.Contains(a, b) {
		return 0.85
	}

	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	matches := 0
	for _, w := range wordsA {
		if setB[w] {
			matches++
		}
	}

	if matches == 0 {
		return 0.35
	}

	total := len(wordsA) + len(wordsB) - matches
	return float64(matches) / float64(total)
}
