package productmatch

// One row, reduced to what a comparison actually reads.
//
// Everything a candidate is scored against is derived here, once per row, and
// then held still while a few hundred candidates are compared to it. That is
// the whole performance argument of the engine: a nine-thousand-row file
// against a twenty-thousand-product catalogue scores millions of pairs, and
// anything recomputed inside that loop is recomputed millions of times.
//
// The scratch space is part of the same argument. The stamp array and the epoch
// counter exist so a comparison can mark which of the row's words a candidate
// consumed without allocating or clearing anything between candidates.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// query is one row reduced to its matching signals, with the scratch space the
// scorer reuses across every candidate so a comparison allocates nothing.
type query struct {
	tokens []string
	// weights is the inverse document frequency of each distinct query token,
	// parallel to tokens; pos indexes back into both.
	weights     []float64
	pos         map[string]int
	totalWeight float64
	// keyPos maps a token's VARIANT key onto the same weight slot its exact
	// spelling occupies, so a candidate carrying another spelling of the word
	// is credited through the primary channel rather than falling through to a
	// discounted one. See variants.go.
	//
	// A key that two different query tokens fold onto is dropped rather than
	// pointing at one of them arbitrarily: crediting the weight of the wrong
	// word is worse than crediting nothing.
	keyPos map[string]int
	// stamp and epoch mark the tokens consumed by the candidate under
	// comparison, so the map never has to be cleared between candidates.
	stamp []int32
	epoch int32

	// distinct marks, per weight slot, whether that word can identify a product
	// on its own — that is, whether the catalogue carries it rarely enough to
	// mean something. Parallel to weights.
	//
	// It is what separates a real match from the failure this file was reopened
	// for: a supplier line and a catalogue entry sharing nothing but "باكت" or
	// "فيتامين" are not the same product, and every measure of overall
	// similarity says otherwise because the shared word is a real, whole,
	// exactly-equal word. Rarity weighting alone only makes such a match score
	// low; it does not stop it being offered, and a low score offered as a
	// suggestion is what the review screen filled up with.
	distinct []bool
	// distinctWeight is the total weight of the distinctive words, which is the
	// denominator the evidence test is measured against.
	distinctWeight float64
	// mods are the row's line-extension keys, compared against the candidate's.
	mods map[string]struct{}
	// rawName is the row's text as written, kept only so a modifier the
	// candidate carries can be looked for unspaced in it before the difference
	// is called a conflict. See modifierGlued.
	rawName string

	// keys are the query tokens' variant keys, used for retrieval. The
	// comparison-side map is keyPos above; this is the list retrieval walks.
	keys     []string
	tri      []trigram
	skeleton string
	nums     []float64
	nameKey  string
	formKey  string
	// subForm separates the semi-solid topicals formKey groups together, so a
	// row asking for a cream is not settled on the same brand's ointment. See
	// topicalSubForm.
	subForm string
	// qty is what the row's figures turned out to be: the packaging counts,
	// each tied to what it counts, and the figures that named nothing. See
	// readQuantities.
	qty quantities
	// marks are the single letters the row carries as identity: the N of
	// Betnovate N. See identityMarks.
	marks    map[string]struct{}
	strength strength
	// strengths is every dose the row states. See MasterProduct.strengths.
	strengths []strength
	packSize  int
	makerKey  string
	sciKey    string
	// priceMinor is the printed public price the row states, in minor units.
	// See evidence_price.go.
	priceMinor int64
}

func (idx *Index) newQuery(row *Row) *query {
	// The distributor's sale terms are cut off before anything reads the name.
	// See annotations.go: what follows them is a price and a carton count, and
	// read as part of the product they are an identity letter and a stray
	// figure that no catalogue entry can agree with.
	name := StripTradeAnnotations(row.Name)
	nameEN := StripTradeAnnotations(row.NameEN)
	full := name + " " + nameEN
	nameTokens := coreTokens(full)
	q := &query{
		tokens:     coreTokens(full + " " + row.Scientific),
		tri:        sortedTrigrams(nameTokens),
		skeleton:   skeletonOf(nameTokens),
		nums:       numberSignature(full),
		nameKey:    strings.Join(nameTokens, " "),
		formKey:    rowFormKey(full, row.DosageForm),
		subForm:    rowSubForm(full, row.DosageForm),
		qty:        readQuantities(full),
		marks:      identityMarks(full),
		strength:   parseStrength(full + " " + row.Concentration),
		strengths:  strengthSet(full + " " + row.Concentration),
		packSize:   row.PackSize,
		makerKey:   sheet.NormalizeKey(row.Manufacturer),
		priceMinor: row.PublicPrice.Minor(),
		sciKey:     sheet.NormalizeKey(row.Scientific),
		mods:       modifiersIn(full),
		rawName:    full,
	}
	if q.packSize == 0 {
		q.packSize = InferPackSize(full)
	}

	// A row that is nothing but company names has no other vocabulary to fall
	// back on, so the demotion below is suspended for it: demoting every word
	// leaves nothing to match on at all.
	onlyMakers := true
	for _, t := range q.tokens {
		if !idx.isCompanyWord(t) {
			onlyMakers = false
			break
		}
	}

	q.pos = make(map[string]int, len(q.tokens))
	for _, t := range q.tokens {
		if _, dup := q.pos[t]; dup {
			continue
		}
		w := idx.idf(t)
		if w <= 0 {
			// A word the catalogue has never seen is the MOST distinctive word
			// in the query, not the least: it is usually the brand, spelled the
			// way this supplier spells it. Weighting it 0.5 — well under the
			// ~2.4 an ordinary "مجم" earns — inverted the whole point of rarity
			// weighting and let the boilerplate outvote the brand. It gets the
			// ceiling instead, so a candidate that lacks it is scored down hard.
			w = idx.maxIDF()
		}
		// A company name is not a product name.
		//
		// Egyptian price lists append the agent to the line — "اوركابريد
		// قطرة/اوركيديا", "ديبوبن امبول/المهن", "تانتم اخضر غرغرة/ابيكو" — and
		// the catalogue records it in its own column instead. Counted as
		// ordinary vocabulary it is a rare word no candidate can carry, so it
		// drags the row's own brand down with it: several plainly correct
		// matches in the live corpus arrived at 0.38 for no other reason.
		//
		// Demoted rather than dropped, because a house brand is sometimes
		// genuinely both, and because the manufacturer is compared properly
		// elsewhere as a field of its own.
		company := !onlyMakers && idx.isCompanyWord(t)
		if company {
			w *= makerTokenWeight
		}
		q.pos[t] = len(q.weights)
		q.weights = append(q.weights, w)
		q.totalWeight += w

		// A word the catalogue has never seen counts as distinctive by the same
		// argument that gives it the ceiling weight: it is the brand, spelled
		// this supplier's way.
		//
		// A company name is demoted in WEIGHT above but is not disqualified from
		// identifying a product, and those are two different questions. The
		// demotion protects a row from being dragged down by an agent's name no
		// candidate carries; distinctiveness asks what an AGREEING word proves.
		// Folding them together cost a whole class of match: اوروفيكس is a rare
		// word in this catalogue's product names and also folds onto a company
		// name, so a row reading "اوروفيكس سموكر" had no distinctive vocabulary
		// at all, was held to the 0.72 blind bar meant for transliteration, and
		// was refused at 0.63 against the one Orovex Smokers there is.
		//
		// So a company word still counts where the catalogue uses it as a
		// product word too. One the catalogue names no product with cannot
		// identify one and stays out, which is the case the demotion was
		// written for.
		distinctive := !idx.commonWord(t) && len([]rune(t)) >= minLinkRunes
		if company && idx.df[t] == 0 {
			distinctive = false
		}
		q.distinct = append(q.distinct, distinctive)
		if distinctive {
			q.distinctWeight += w
		}
	}
	q.stamp = make([]int32, len(q.weights))
	q.keyPos = buildKeyPos(q.pos)
	q.keys = variantKeys(q.tokens)
	return q
}

// makerTokenWeight is what a company name is worth as identifying vocabulary,
// as a fraction of what the same word would be worth otherwise.
//
// Low, because the manufacturer is compared as its own field and a row that
// names it has said nothing about which of that company's products it is. Not
// zero, because Egyptian house brands — ستار فيل, سترونج فيل — are the company
// and the product line at once, and zeroing the word would leave those rows
// with no vocabulary at all.
const makerTokenWeight = 0.30

// buildKeyPos maps each query token's variant key onto that token's weight slot.
//
// A key two different query tokens fold onto is left out entirely rather than
// pointing at whichever was seen first: crediting one word's weight to another
// word's spelling is a wrong answer, and no answer is better than a wrong one
// in the channel that decides most matches.
func buildKeyPos(pos map[string]int) map[string]int {
	keyPos := make(map[string]int, len(pos))
	shared := make(map[string]struct{})
	for token, slot := range pos {
		key := variantKeyOf(token)
		if key == "" || key == token {
			continue
		}
		if _, dup := shared[key]; dup {
			continue
		}
		if existing, taken := keyPos[key]; taken && existing != slot {
			delete(keyPos, key)
			shared[key] = struct{}{}
			continue
		}
		keyPos[key] = slot
	}
	return keyPos
}

// rowFormKey is the pharmaceutical form a row states, the NAME first.
//
// Same order, and the same reason, as MasterProduct.formOf: the name is what a
// person wrote and the form column is what an importer filled in. Reading the
// two as one string made the answer depend on the order of the form vocabulary
// rather than on the row — a name saying "30 قرص" beside a column saying "جل"
// resolved to a gel, and a tablet whose name could not be plainer was refused
// against the tablet in the catalogue. The column was itself inferred from the
// name by a substring test that found جل inside جليماديل, so the two came from
// the same text and only one of them was right.
//
// The column is still read where the name says nothing, which is the case it
// exists for: a supplier file with a real dosage-form column and bare brand
// names beside it.
func rowFormKey(name, column string) string {
	if key := formKeyOf(name); key != "" {
		return key
	}
	return formKeyOf(column)
}

// rowSubForm reads the topical sub-form the same way round, for the same
// reason.
func rowSubForm(name, column string) string {
	if sub := topicalSubForm(name); sub != "" {
		return sub
	}
	return topicalSubForm(column)
}
