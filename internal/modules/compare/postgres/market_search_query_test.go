package postgres

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

// A word is a substring of the name. The wildcards must sit outside
// platform.normalize_arabic, because that function turns '%' into a space and
// trims it: a pattern normalised whole matches only a name equal to the word.
func TestMarketSearchKeepsWordWildcardsOutsideNormalisation(t *testing.T) {
	clause, args := marketSearchFilter(compare.MarketDiscountsFilter{Query: "بانادول اكسترا"})

	const wrapped = "LIKE ('%' || lower(platform.normalize_arabic($"
	// One per word, in the row terms and again in the catalogue terms.
	if got := strings.Count(clause, wrapped); got != 4 {
		t.Fatalf("wildcard-wrapped patterns = %d, want 4 (two words x row and catalogue)\n%s", got, clause)
	}
	if strings.Contains(clause, "LIKE lower(platform.normalize_arabic(") {
		t.Fatal("a pattern is normalised before its wildcards are added; the wildcards are lost")
	}

	// The words arrive raw. Escaping them before normalisation would leave
	// backslashes behind, which normalisation turns into spaces.
	if len(args) != 4 {
		t.Fatalf("args = %d, want query, two words and the catalogue ids", len(args))
	}
	if args[1] != "بانادول" || args[2] != "اكسترا" {
		t.Fatalf("words bound as %q and %q, want them unchanged", args[1], args[2])
	}
}

// Characters a pharmacist types as wildcards are passed through unchanged;
// normalisation in the database removes them from the comparison.
func TestMarketSearchPassesLikeMetacharactersThroughUnescaped(t *testing.T) {
	_, args := marketSearchFilter(compare.MarketDiscountsFilter{Query: `50%_\`})
	if len(args) < 2 || args[1] != `50%_\` {
		t.Fatalf("word bound as %v, want the raw text", args)
	}
}
