package productmatch

// The catalogue side of a comparison, derived once.
//
// A MasterProduct is a catalogue product projected down to the fields matching
// actually reads, plus everything the scorer would otherwise recompute. Holding
// twenty thousand of these costs a few megabytes; holding twenty thousand full
// products would not, and recomputing one product's trigrams inside the scoring
// loop is the difference between two seconds and forty on a real file.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// MasterProduct is the catalogue side of a match, projected down to the fields
// matching actually reads. Holding thirty thousand of these costs a few
// megabytes; holding thirty thousand full products would not.
type MasterProduct struct {
	ID            int64
	NameAR        string
	NameEN        string
	SKU           string
	Barcode       string
	Scientific    string
	DosageForm    string
	Concentration string
	Unit          string
	Manufacturer  string
	PublicPrice   string

	// Derived once at index time. Everything the scorer needs is precomputed
	// here rather than per comparison: a nine-thousand-row file scores three
	// million pairs, and recomputing one product's trigrams inside that loop is
	// the difference between two seconds and forty.
	coreAR []string
	coreEN []string
	// keysAR and keysEN are the variant keys of the core tokens: the fold that
	// makes "ابيكوبريد" and "ابيكوبرايد" the same word, and that lets an Arabic
	// query token meet a Latin catalogue token in the primary channel rather
	// than in a discounted one at the end. See variants.go.
	//
	// They are de-duplicated and are what RETRIEVAL walks. Comparison needs the
	// key of a particular token instead, which is what tokKeyAR/tokKeyEN are:
	// parallel to the core tokens, empty where a word is too short to fold.
	// Recomputing a skeleton inside the comparison loop would be the same
	// answer at three million times the price.
	keysAR   []string
	keysEN   []string
	tokKeyAR []string
	tokKeyEN []string
	// wAR and wEN are the rarity weights of the core tokens, parallel to them,
	// zero for a token the same name already carried. totAR and totEN are their
	// sums.
	//
	// They exist so a comparison can ask the question the scorer was missing:
	// not only "how much of the row did the catalogue entry account for" but
	// "how much of the CATALOGUE ENTRY did the row account for". A row matching
	// only the descriptive half of a product name — "غسول نسائى 200 مل" against
	// another company's "غسول نسائي 200 مل" — scores well on the first question
	// and badly on the second, and the second is the one that was not asked.
	wAR     []float64
	wEN     []float64
	totAR   float64
	totEN   float64
	triAR   []trigram
	triEN   []trigram
	nums    []float64
	nameKey string
	// formKey is the form BOTH names between them state, which is what the
	// scorer's corroboration bonus and the AI guard's veto read. The per-side
	// reductions in factsAR/factsEN are what the conflict comparison reads —
	// see facts.go for why those must not be pooled.
	formKey string
	// formMeta is the form the record's own dosage-form column states, used
	// only where a name states none.
	formMeta string
	// factsAR and factsEN are each name reduced to the attributes it states.
	factsAR  nameFacts
	factsEN  nameFacts
	strength strength
	// strengthsMeta are the doses the record's concentration column states,
	// used only where a name states none. See nameFacts.strengths.
	strengthsMeta []strength
	// strengths is every dose the record states, not merely the first.
	//
	// The scorer used to compare one against one, which made two unrelated
	// measurements look like a contradiction: "ايفرزين 30 جم كريم" states a
	// tube size and the catalogue's "ايفرزين 1% كريم 30 جم" states a
	// concentration, and 30 g against 1% was scored as a dose conflict and cost
	// the row 0.45 — the same product, refused. The identity guard has always
	// compared per unit; the scorer did not, and this is what lets it.
	strengths []strength
	packSize  int
	makerKey  string
	sciKey    string
	// priceMinor is the printed public price in minor units, zero where the
	// record has none. See evidence_price.go.
	priceMinor int64
	// makerTokens are the words of this product's manufacturer field, which
	// seed the index's company vocabulary. See Index.makers.
	makerTokens []string
	// mods are the line-extension keys this product's name carries — the words
	// that make "بانادول اكسترا" a different product from "بانادول". Derived
	// once here because the deterministic scorer now consults them on every
	// comparison, not only the AI guard, and re-tokenising a catalogue name
	// three million times is not a cheaper way to learn the same thing.
	mods map[string]struct{}
	// skeleton is the consonant reduction both scripts fold into, which is the
	// only channel on which an Arabic query and a Latin catalogue name can
	// agree at all. See translit.go.
	skeleton string
}

// strength is a parsed dose: a number and the unit it was written in, converted
// to a common base so 1g and 1000mg compare equal.
type strength struct {
	value float64
	unit  string
	// parts is how many figures the ratio this dose came from carried: 1 for a
	// plain "500 مجم", 2 for the "32/25 مجم" of a combination.
	//
	// It is what tells اتاكاند from اتاكاند بلس without reading the name's
	// grammar. Counting the doses per UNIT instead does not work: "2.5مجم/5مل
	// شراب 100 مل" states two millilitre figures for reasons that have nothing
	// to do with combination, and comparing those counts made a product
	// contradict itself written two ways.
	parts int
}

func (s strength) known() bool { return s.value > 0 }

// prepare derives a product's matching keys.
func prepare(p *MasterProduct) {
	p.coreAR = coreTokens(p.NameAR)
	p.coreEN = coreTokens(p.NameEN)
	p.keysAR = variantKeys(p.coreAR)
	p.keysEN = variantKeys(p.coreEN)
	p.tokKeyAR = tokenVariantKeys(p.coreAR)
	p.tokKeyEN = tokenVariantKeys(p.coreEN)
	p.triAR = sortedTrigrams(p.coreAR)
	p.triEN = sortedTrigrams(p.coreEN)
	p.nums = numberSignature(p.NameAR + " " + p.NameEN)
	p.nameKey = strings.Join(p.coreAR, " ")
	if p.nameKey == "" {
		p.nameKey = strings.Join(p.coreEN, " ")
	}
	full := p.NameAR + " " + p.NameEN + " " + p.DosageForm
	p.formKey = formKeyOf(full)
	p.formMeta = formKeyOf(p.DosageForm)
	p.factsAR = factsOf(p.NameAR)
	p.factsEN = factsOf(p.NameEN)
	p.strength = parseStrength(p.NameAR + " " + p.NameEN + " " + p.Concentration)
	p.strengths = strengthSet(p.NameAR + " " + p.NameEN + " " + p.Concentration)
	p.strengthsMeta = strengthSet(p.Concentration)
	p.packSize = InferPackSize(p.NameAR + " " + p.NameEN)
	p.makerTokens = coreTokens(p.Manufacturer)
	p.makerKey = sheet.NormalizeKey(p.Manufacturer)
	p.priceMinor = parsePriceMinor(p.PublicPrice)
	p.sciKey = sheet.NormalizeKey(p.Scientific)
	p.mods = modifiersIn(p.NameAR + " " + p.NameEN)
	// Built from whichever name the catalogue actually holds — both, where it
	// holds both — because the query may be written in either script and the
	// skeleton is the same either way.
	p.skeleton = skeletonOf(append(append([]string{}, p.coreAR...), p.coreEN...))
}
