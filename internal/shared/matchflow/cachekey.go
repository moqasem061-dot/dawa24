package matchflow

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// The decision cache's key.
//
// Two importers asking the same question must land on the same row of
// catalog.match_decisions, or the cache does nothing: an answer paid for by a
// pharmacy's order has to be free to the vendor whose price list asks it, and
// the reverse. That is the entire economic argument for caching at all.
//
// The two implementations of this were byte-identical, which is not the same as
// being the same. They agreed by coincidence, in two files nobody diffs, keyed
// on a constant that had already drifted once — and a one-character difference
// in either would have split the cache again silently, because a cache that
// misses looks exactly like a cache that is cold.
//
// So it is computed here, once, and both callers hash the same bytes by
// construction rather than by inspection.

// unitSeparator delimits the key's fields. A control character rather than a
// comma or a colon, because a product name may contain either and a delimiter
// that appears in the data is not a delimiter.
const unitSeparator = '\x1f'

// DecisionKey identifies one question: this text, against these candidates,
// under this prompt.
//
// All three parts matter. The same text against a different shortlist is a
// different question — the model can only answer from what it was shown — and
// the same question under a different prompt may get a different answer, which
// is why the version is in the key rather than merely recorded beside it.
//
// candidates need not be sorted or de-duplicated; both are done here, so a
// caller that retrieved the same products in a different order still hits.
func DecisionKey(normName string, candidates []int64) string {
	ids := make([]int64, len(candidates))
	copy(ids, candidates)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var b strings.Builder
	b.Grow(len(normName) + len(ids)*8 + len(PromptVersion) + 2)
	b.WriteString(normName)
	b.WriteByte(unitSeparator)

	var last int64 = -1
	for i, id := range ids {
		if i > 0 && id == last {
			continue
		}
		last = id
		b.WriteString(strconv.FormatInt(id, 10))
		b.WriteByte(',')
	}
	b.WriteByte(unitSeparator)
	b.WriteString(PromptVersion)

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
