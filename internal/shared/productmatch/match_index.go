package productmatch

import (
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Product matching.
//
// A vendor's row has to be tied to a product in the shared catalogue, because
// that is what makes their offer comparable with anyone else's. The name they
// wrote is not the name in the catalogue: it is abbreviated, transliterated
// differently, carries the pack count and the word "سعر جديد", and was typed by
// a different person on a different keyboard.
//
// The index below is built once per import and answers, for each row, which
// catalogue products could plausibly be it. The scoring that follows decides
// which one, and — as importantly — when it should refuse to decide.

// Index is the in-memory catalogue, keyed several ways.
type Index struct {
	products  []*MasterProduct
	byID      map[int64]*MasterProduct
	byBarcode map[string][]*MasterProduct
	bySKU     map[string][]*MasterProduct
	byName    map[string][]*MasterProduct
	// tokens maps a significant word to the products whose name contains it.
	tokens map[string][]*MasterProduct
	// keyTokens maps a VARIANT key to the same products.
	//
	// Without it the variant channel could never fire: retrieval is what
	// decides which products are scored at all, and a query whose brand is
	// spelled differently from the catalogue's shares no exact token with it,
	// so the pool came back empty and the scorer was never asked. The fold has
	// to happen at retrieval as well as at comparison, or it happens nowhere.
	keyTokens map[string][]*MasterProduct
	// df is the document frequency of each token, which is what makes a rare
	// brand name count for more than the word "شراب".
	df map[string]int
	// makers is every word that appears in some product's manufacturer field.
	//
	// It is drawn from the catalogue rather than from a list, because the
	// companies that matter are the ones this catalogue actually names. It is
	// built eagerly, unlike the retrieval indexes, because the SCORER reads it
	// on every row: Egyptian price lists write the agent after a slash —
	// "اوركابريد قطرة/اوركيديا", "ديبوبن امبول/المهن" — and a company name
	// carried by one side and not the other is not a difference between two
	// products. Counting it as one is how a correct match arrived at 0.39.
	makers map[string]bool
	// makerKeys is the same vocabulary folded onto its consonant skeleton.
	//
	// It is what makes the company check work at all on this catalogue: the
	// manufacturer column is written in Latin ("orchidia", "hikma") and the
	// price list writes the same company in Arabic ("اوركيديا", "هيكما"). Two
	// spellings, one company, and no exact word in common — so a set of the raw
	// words recognised nothing. The skeleton folds both scripts onto one key,
	// which is the same mechanism the scorer's variant channel uses.
	makerKeys map[string]bool
	total     int
	// tri holds the trigram and scientific-name posting lists that only Recall
	// needs, built lazily. Most runs never reach the AI stage, and an index
	// nothing reads should not be paid for on every import.
	tri recallIndex
}

// NewIndex builds the matching index. Ownership of the slice passes to it.
func NewIndex(products []MasterProduct) *Index {
	idx := &Index{
		byID:      make(map[int64]*MasterProduct, len(products)),
		byBarcode: make(map[string][]*MasterProduct),
		bySKU:     make(map[string][]*MasterProduct),
		byName:    make(map[string][]*MasterProduct),
		tokens:    make(map[string][]*MasterProduct, len(products)),
		keyTokens: make(map[string][]*MasterProduct, len(products)),
		df:        make(map[string]int, len(products)),
		makers:    make(map[string]bool, 2048),
		makerKeys: make(map[string]bool, 2048),
	}

	// One scratch buffer for the whole build, reused per product.
	scratch := make([]string, 0, 16)

	for i := range products {
		p := &products[i]
		if p.ID <= 0 {
			continue
		}
		prepare(p)
		for _, tok := range p.makerTokens {
			idx.makers[tok] = true
			if key := variantKeyOf(tok); key != "" {
				idx.makerKeys[key] = true
			}
		}
		idx.products = append(idx.products, p)
		idx.byID[p.ID] = p
		idx.total++

		if code := sheet.DigitsOnly(p.Barcode); code != "" {
			idx.byBarcode[code] = append(idx.byBarcode[code], p)
		}
		if key := sheet.NormalizeCode(p.SKU); key != "" {
			idx.bySKU[key] = append(idx.bySKU[key], p)
		}
		if p.nameKey != "" {
			idx.byName[p.nameKey] = append(idx.byName[p.nameKey], p)
		}
		if enKey := strings.Join(p.coreEN, " "); enKey != "" && enKey != p.nameKey {
			idx.byName[enKey] = append(idx.byName[enKey], p)
		}

		// Dedup by scanning the handful of tokens already taken, not by
		// building a map per product. A product name holds two or three
		// identifying words; a map for that costs an allocation and a hash per
		// lookup to deduplicate a list short enough to scan. Across a hundred
		// and fifty thousand products it was three hundred thousand maps.
		scratch = scratch[:0]
		for _, tok := range p.coreAR {
			scratch = appendUnique(scratch, tok)
		}
		for _, tok := range p.coreEN {
			scratch = appendUnique(scratch, tok)
		}
		for _, tok := range scratch {
			idx.tokens[tok] = append(idx.tokens[tok], p)
			idx.df[tok]++
		}

		// The variant postings. Keyed separately from the exact ones because
		// the two must not share a document frequency: a fold is by design
		// carried by more products than any one spelling, and letting that
		// inflate df would make every word look common and flatten the rarity
		// weighting the scorer depends on.
		scratch = scratch[:0]
		for _, key := range p.keysAR {
			scratch = appendUnique(scratch, key)
		}
		for _, key := range p.keysEN {
			scratch = appendUnique(scratch, key)
		}
		for _, key := range scratch {
			idx.keyTokens[key] = append(idx.keyTokens[key], p)
		}
	}

	// Token weights are a second pass because they are inverse document
	// frequencies: no product's weights are knowable until every product has
	// been counted.
	for _, p := range idx.products {
		p.wAR, p.totAR = idx.tokenWeights(p.coreAR)
		p.wEN, p.totEN = idx.tokenWeights(p.coreEN)
	}
	return idx
}

// tokenWeights is the rarity weight of each token in a name, with a repeated
// token weighted once so a name that says the same word twice is not counted
// twice for it.
func (idx *Index) tokenWeights(tokens []string) ([]float64, float64) {
	if len(tokens) == 0 {
		return nil, 0
	}
	out := make([]float64, len(tokens))
	var total float64
	for i, t := range tokens {
		duplicate := false
		for j := 0; j < i; j++ {
			if tokens[j] == t {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		w := idx.idf(t)
		if w <= 0 {
			w = idx.maxIDF()
		}
		out[i] = w
		total += w
	}
	return out, total
}

// isCompanyWord reports whether a word names a manufacturer this catalogue
// knows, in either script.
func (idx *Index) isCompanyWord(token string) bool {
	if idx == nil || token == "" {
		return false
	}
	if idx.makers[token] {
		return true
	}
	key := variantKeyOf(token)
	return key != "" && idx.makerKeys[key]
}

// appendUnique adds a token if the short list does not already carry it.
func appendUnique(list []string, token string) []string {
	for _, existing := range list {
		if existing == token {
			return list
		}
	}
	return append(list, token)
}

// Size is how many products the index holds.
func (idx *Index) Size() int { return idx.total }

// Name returns the catalogue's own label for a product, Arabic first.
//
// It exists so a caller that has resolved a match can report *which* product it
// resolved to without a second query: the index already holds every name it
// scored against. A results table or an import ledger that prints only the id
// has told the reader nothing they can check.
func (idx *Index) Name(id int64) string {
	if idx == nil {
		return ""
	}
	p, ok := idx.byID[id]
	if !ok {
		return ""
	}
	if p.NameAR != "" {
		return p.NameAR
	}
	return p.NameEN
}

// Lookup returns the indexed projection of one catalogue product.
func (idx *Index) Lookup(id int64) (*MasterProduct, bool) {
	if idx == nil {
		return nil, false
	}
	p, ok := idx.byID[id]
	return p, ok
}

// maxIDF is the weight of a token no catalogue product carries — the ceiling
// the idf curve approaches as a word gets rarer.
func (idx *Index) maxIDF() float64 {
	return math.Log(float64(idx.total + 1))
}

// idf weights a token by how rare it is in the catalogue.
func (idx *Index) idf(token string) float64 {
	n := idx.df[token]
	if n <= 0 {
		return 0
	}
	return math.Log(float64(idx.total+1) / float64(n+1))
}

// candidatePool gathers the products worth scoring for one query.
//
// Rare tokens are consulted first and the pool is capped, which is what keeps
// matching linear in the file rather than quadratic against the catalogue: a
// query for "بانادول اكسترا" reads two postings lists rather than thirty
// thousand products.
func (idx *Index) candidatePool(tokens []string, limit int) []*MasterProduct {
	return idx.candidatePoolWithKeys(tokens, nil, limit)
}

// candidatePoolWithKeys gathers the products worth scoring, retrieving by exact
// word first and by variant key for whatever the words did not find.
//
// The order is the whole design. Exact postings are consulted first and the
// pool is filled from them, so a query whose words the catalogue actually
// carries costs exactly what it did before. The variant postings are read only
// when that left room — which is precisely the case the fold exists for: the
// brand is spelled differently, no exact posting exists, and the pool would
// otherwise have come back empty and the scorer never been asked.
func (idx *Index) candidatePoolWithKeys(tokens, keys []string, limit int) []*MasterProduct {
	if len(tokens) == 0 && len(keys) == 0 {
		return nil
	}
	ordered := make([]string, len(tokens))
	copy(ordered, tokens)
	sort.SliceStable(ordered, func(i, j int) bool { return idx.idf(ordered[i]) > idx.idf(ordered[j]) })

	// A token carried by a large share of the catalogue tells us nothing and
	// costs everything: adding its postings list drags two thousand products
	// into a comparison the rarest token has already answered.
	crowded := idx.total / 20
	if crowded < 64 {
		crowded = 64
	}

	pool := make(map[int64]*MasterProduct, limit)
	for _, tok := range ordered {
		postings := idx.tokens[tok]
		if len(postings) > crowded && len(pool) > 0 {
			continue
		}
		if takeInto(pool, postings, limit) {
			break
		}
	}
	// Variant retrieval, for what the exact words did not reach.
	if len(pool) < limit && len(keys) > 0 {
		for _, key := range keys {
			postings := idx.keyTokens[key]
			// A fold carried by a large share of the catalogue is a fold that
			// has stopped identifying anything, and dragging its postings in
			// would swamp the pool with products the exact channel already
			// ruled out.
			if len(postings) > crowded && len(pool) > 0 {
				continue
			}
			if takeInto(pool, postings, limit) {
				break
			}
		}
	}

	out := make([]*MasterProduct, 0, len(pool))
	for _, p := range pool {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// takeInto adds a postings list to the pool, stopping at the limit. It reports
// whether the pool is now full.
//
// The limit is checked INSIDE the loop, and that is the whole point of this
// existing. The crowded guard above only skips an over-large postings list when
// the pool already holds something, so the first token consulted was exempt
// from every bound there is: a query whose rarest word happens to be carried by
// most of the catalogue pulled the entire catalogue into one row's comparison.
// At a hundred and fifty thousand products and thirty thousand rows that is
// four and a half billion scored pairs, which is not a slow import — it is one
// that never finishes.
//
// Taking a prefix of a very common word's postings is arbitrary, and it is
// supposed to be. The guard only fires for a word carried by more than a
// twentieth of the catalogue, which by construction is a word that identifies
// almost nothing; the rows it would have settled are settled by the rarer words
// beside it, and the ones with no rarer word are exactly the rows that belong
// in front of a human anyway.
func takeInto(pool map[int64]*MasterProduct, postings []*MasterProduct, limit int) bool {
	for _, p := range postings {
		if len(pool) >= limit {
			return true
		}
		pool[p.ID] = p
	}
	return len(pool) >= limit
}
