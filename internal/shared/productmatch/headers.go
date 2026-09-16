package productmatch

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Header match strengths. The gaps are wide on purpose: an exact header match
// must beat any number of partial ones, and a full multi-word term a field owns
// must beat a single word several fields share.
const (
	scoreExact  = 100
	scoreStrong = 60
	scoreWeak   = 25
	scoreFloor  = scoreWeak
)

// headerSpec is how one field is recognised in a header cell.
//
// exact matches the whole normalised header. strong and weak are substring
// tests. blocked disqualifies the pair outright, which is how "سعر التكلفة" is
// kept away from the public price and "الباركود الدولي" away from the item
// code — the two collisions that quietly corrupt an imported price list.
type headerSpec struct {
	field   Field
	exact   []string
	strong  []string
	weak    []string
	blocked []string
}

// foldedSpecs is headerSpecs with every synonym run through the same normaliser
// the header cells go through, so a match is a plain string comparison and
// neither side can drift.
var foldedSpecs = func() []headerSpec {
	fold := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]bool{}
		for _, s := range in {
			key := sheet.NormalizeKey(s)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
		return out
	}
	out := make([]headerSpec, len(headerSpecs))
	for i, spec := range headerSpecs {
		out[i] = headerSpec{
			field:   spec.field,
			exact:   fold(spec.exact),
			strong:  fold(spec.strong),
			weak:    fold(spec.weak),
			blocked: fold(spec.blocked),
		}
	}
	return out
}()

// scoreHeader rates how well one normalised header cell names one field, and
// reports whether the pair was disqualified outright.
func scoreHeader(spec headerSpec, key string) (score int, blocked bool) {
	if key == "" {
		return 0, false
	}
	for _, b := range spec.blocked {
		if strings.Contains(key, b) {
			return 0, true
		}
	}
	for _, e := range spec.exact {
		if key == e {
			return scoreExact, false
		}
	}
	best := 0
	for _, s := range spec.strong {
		if strings.Contains(key, s) {
			// A longer matched phrase is stronger evidence than a shorter one,
			// which is what separates "سعر البيع للجمهور" from a bare "سعر".
			if v := scoreStrong + len([]rune(s)); v > best {
				best = v
			}
		}
	}
	if best > 0 {
		return best, false
	}
	for _, w := range spec.weak {
		if strings.Contains(key, w) {
			if v := scoreWeak + len([]rune(w))/2; v > best {
				best = v
			}
		}
	}
	return best, false
}

// HeaderEvidence is what the synonym table made of one column for one field.
type HeaderEvidence struct {
	Score   int
	Blocked bool
}

// headerEvidence scores one header cell against every field.
func headerEvidence(header string) map[Field]HeaderEvidence {
	key := sheet.NormalizeKey(header)
	out := make(map[Field]HeaderEvidence, len(foldedSpecs))
	for _, spec := range foldedSpecs {
		score, blocked := scoreHeader(spec, key)
		if score == 0 && !blocked {
			continue
		}
		out[spec.field] = HeaderEvidence{Score: score, Blocked: blocked}
	}
	return out
}
