package productmatch

import (
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// trigram is a three-character window, hashed.
//
// Hashed rather than kept as a string, and the difference is not academic. A
// catalogue of a hundred and fifty thousand products holds about three million
// trigram instances across both of its names; as strings that is three million
// heap allocations at index time and a slice header plus a backing array for
// every one of them. Measured, the string version built its index in 2.2
// seconds and allocated 450 MB across 8.9 million allocations, and almost all
// of it was here.
//
// A trigram is never read back — nothing prints one, nothing logs one, the sets
// exist only to be intersected — so nothing is lost by keeping the hash instead
// of the text. Sixty-four bits makes a collision between two distinct trigrams
// vanishingly unlikely across any catalogue this will ever hold, and a
// collision would in any case only perturb a similarity channel that is already
// discounted below a shared whole word.
type trigram uint64

// sortedTrigrams is the deduplicated, ordered trigram set of a token list, so
// two sets can be intersected by a two-pointer walk with no allocation.
func sortedTrigrams(tokens []string) []trigram {
	if len(tokens) == 0 {
		return nil
	}
	return trigramsOf(strings.Join(tokens, ""))
}

// trigramsOf hashes every three-rune window of one string.
func trigramsOf(s string) []trigram {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) < 3 {
		return []trigram{hashRunes(runes)}
	}
	out := make([]trigram, 0, len(runes)-2)
	for i := 0; i <= len(runes)-3; i++ {
		out = append(out, hashRunes(runes[i:i+3]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	kept := out[:1]
	for _, t := range out[1:] {
		if t != kept[len(kept)-1] {
			kept = append(kept, t)
		}
	}
	return kept
}

// hashRunes is FNV-1a over the runes of one window.
func hashRunes(runes []rune) trigram {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for _, r := range runes {
		u := uint32(r)
		for shift := 0; shift < 32; shift += 8 {
			h ^= uint64(byte(u >> shift))
			h *= prime64
		}
	}
	return trigram(h)
}

// jaccardSorted is the overlap of two sorted, deduplicated trigram sets.
func jaccardSorted(a, b []trigram) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// numberSignature is the set of plain numbers a product name carries, sorted.
//
// It is what separates "ستار فيل مناديل ميسيلار 25 منديل" from the same product
// in a fifty-wipe pack. The identifying words are identical, the dose is absent
// from both, and without the count the two score the same and the engine can
// only report that it cannot choose.
func numberSignature(name string) []float64 {
	fields := strings.FieldsFunc(sheet.NormalizeDigits(name), func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '.'
	})
	var out []float64
	for _, f := range fields {
		v, err := strconv.ParseFloat(strings.Trim(f, "."), 64)
		if err != nil || v <= 0 || v > 100000 {
			continue
		}
		out = append(out, v)
	}
	sort.Float64s(out)
	// Deduplicate: "1+1_60ملى" repeating a figure says nothing extra.
	if len(out) > 1 {
		kept := out[:1]
		for _, v := range out[1:] {
			if v != kept[len(kept)-1] {
				kept = append(kept, v)
			}
		}
		out = kept
	}
	return out
}

// DebugCoreTokens exposes the identifying words of a name, for diagnostics.
func DebugCoreTokens(name string) []string { return coreTokens(name) }
