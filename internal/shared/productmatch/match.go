package productmatch

import (
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Match scoring.
//
// Every signal contributes, and two of them can veto. A dose that disagrees and
// a form that disagrees are not weak evidence against a match — they are proof
// it is a different product, and a name similarity of 0.95 between "أوجمنتين
// 1جم" and "أوجمنتين 625مجم" is exactly the case where the name is the least
// reliable thing in the row.

// MatchLevel says how a row was tied to a catalogue product.
type MatchLevel string

const (
	// MatchBarcode is a GTIN hit: the same physical package.
	MatchBarcode MatchLevel = "barcode"
	// MatchCode is a supplier item code that the catalogue also carries.
	MatchCode MatchLevel = "code"
	// MatchExact is an identical name once folded, with no attribute conflict.
	MatchExact MatchLevel = "exact"
	// MatchStrong is a high-scoring similarity match with corroborating
	// attributes.
	MatchStrong MatchLevel = "strong"
	// MatchReview is plausible and must be confirmed by the vendor.
	MatchReview MatchLevel = "review"
	// MatchAmbiguous means two catalogue products fit equally well.
	MatchAmbiguous MatchLevel = "ambiguous"
	// MatchNone means nothing in the catalogue fits.
	MatchNone MatchLevel = "none"
)

// Label renders a level in Arabic.
func (l MatchLevel) Label() string {
	switch l {
	case MatchBarcode:
		return "مطابقة بالباركود"
	case MatchCode:
		return "مطابقة بالكود"
	case MatchExact:
		return "مطابقة تامة للاسم"
	case MatchStrong:
		return "مطابقة قوية"
	case MatchReview:
		return "تحتاج مراجعة"
	case MatchAmbiguous:
		return "أكثر من صنف محتمل"
	default:
		return "غير مطابق"
	}
}

// Settled reports whether the match may be applied without asking.
func (l MatchLevel) Settled() bool {
	switch l {
	case MatchBarcode, MatchCode, MatchExact, MatchStrong:
		return true
	}
	return false
}

// MatchCandidate is one catalogue product a row could be.
type MatchCandidate struct {
	ProductID     int64   `json:"product_id"`
	Name          string  `json:"name"`
	Scientific    string  `json:"scientific,omitempty"`
	DosageForm    string  `json:"dosage_form,omitempty"`
	Concentration string  `json:"concentration,omitempty"`
	Manufacturer  string  `json:"manufacturer,omitempty"`
	PublicPrice   string  `json:"public_price,omitempty"`
	Score         float64 `json:"score"`
	Reason        string  `json:"reason,omitempty"`
}

// MatchResult is the outcome for one row.
type MatchResult struct {
	ProductID  int64            `json:"product_id,omitempty"`
	Level      MatchLevel       `json:"level"`
	Score      float64          `json:"score"`
	Reason     string           `json:"reason"`
	Candidates []MatchCandidate `json:"candidates,omitempty"`
}

// Matched reports whether a product was tied to the row at all.
func (r MatchResult) Matched() bool { return r.ProductID > 0 }

// MatchOptions tune the decision thresholds.
type MatchOptions struct {
	// MinStrong is the score at or above which a match is applied without
	// asking. Lowering it trades false negatives for false positives, and in a
	// catalogue where a false positive prices the wrong medicine, the default
	// is deliberately conservative.
	MinStrong float64
	// MinReview is the score below which nothing is offered at all.
	MinReview float64
	// TrustSupplierCode allows the file's item code to match the catalogue's.
	//
	// Off by default: a vendor's "951" is their internal numbering and
	// coincides with a catalogue code by accident more often than by design.
	// Set it through WithIdentifiers rather than directly, so it can never be
	// on for a file whose code column was never mapped.
	TrustSupplierCode bool
	// TrustBarcode allows the file's barcode to settle a match on its own.
	//
	// Off by default, and it did not used to be a choice at all: the barcode
	// tier ran unconditionally, before the name, the dose or the form was
	// consulted, and any eight-digit value with a single catalogue hit won at
	// confidence 1.0. Pharmacy item numbering is routinely eight or nine
	// digits, so a file that mapped its own reference numbers into that field
	// produced confident, unreviewable links to unrelated medicines.
	//
	// A GTIN really does identify a package, so when the column holds one this
	// is the strongest evidence available. The question is only ever whether
	// the column holds one, and that is the user's to answer.
	TrustBarcode bool
	// CodeIsAuthoritative accepts a code match without corroboration from the
	// name.
	//
	// Off by default, and the default is right for a supplier file: a vendor's
	// "951" collides with a catalogue code by accident more often than by
	// design, so a bare numeric hit must be seconded by the name.
	//
	// On where the user has said the column *is* the catalogue's code — a
	// pharmacy's own reference list is kept by the code they look products up
	// by, and demanding the name agree as well refuses the exact rows the
	// column was mapped for.
	CodeIsAuthoritative bool
	// PoolLimit caps how many catalogue products are scored per row.
	PoolLimit int
	// MaxCandidates is how many alternatives are kept for the review screen.
	MaxCandidates int
}

// The thresholds every tool starts on.
//
// DefaultMinStrong is stated once, here, because four screens print it and four
// importers compare against it, and a figure that lives in six places is a
// figure that means six things by the end of the year. The vendor catalogue
// import, the admin main-catalogue import, the saving-products import and the
// smart order all begin here; each may raise it, none may lower it past
// DefaultMinReview.
//
// Sixty per cent, at the client's direction. It was fifty, and before that
// forty. The band below sixty is, on the live supplier files,
// predominantly line extensions and coincidences — the same brand at another
// dose, or two products agreeing on a category word — and applying those
// silently is what a vendor discovers three weeks later in their own catalogue.
// Everything below 60% is strictly sent to human review; it is still shown,
// still scored, and still one click from being accepted; it is simply not
// accepted FOR them automatically.
const (
	DefaultMinStrong = 0.60
	// DefaultMinReview is the floor below which nothing is offered at all.
	//
	// It used to be 0.15, which is how a review screen came to be full of
	// sixteen-per-cent suggestions. A score that low is not a weak opinion, it
	// is the absence of one, and printing a product name beside it invites a
	// reviewer to accept something the engine never believed.
	DefaultMinReview = 0.25
)

// DefaultMatchOptions are the thresholds the wizard starts on.
//
// Every identifier tier is off. That is deliberate and it is the important part
// of this function: a tier that settles a match without consulting the name is
// only safe when the user has said the column holds the catalogue's own
// identifier, and a default of "on" asks nobody. Tools switch them on through
// WithIdentifiers once the column is mapped and the user has chosen it.
func DefaultMatchOptions() MatchOptions {
	return MatchOptions{
		MinStrong:     DefaultMinStrong,
		MinReview:     DefaultMinReview,
		PoolLimit:     400,
		MaxCandidates: 5,
	}
}

// Match ties one parsed row to a catalogue product.
func (idx *Index) Match(row *Row, opts MatchOptions) MatchResult {
	if idx == nil || idx.total == 0 {
		return MatchResult{Level: MatchNone, Reason: "الكتالوج المركزي فارغ؛ لا يوجد ما يُطابَق عليه"}
	}
	if opts.PoolLimit <= 0 {
		opts.PoolLimit = 400
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 5
	}

	q := idx.newQuery(row)

	if opts.TrustBarcode {
		if res, ok := idx.matchByBarcode(row); ok {
			return res
		}
	}
	if opts.TrustSupplierCode {
		if res, ok := idx.matchByCode(row, q, opts); ok {
			return res
		}
	}

	scored := idx.score(q, opts)
	if len(scored) == 0 {
		return MatchResult{Level: MatchNone, Reason: "لم يتم العثور على صنف مشابه في الكتالوج المركزي"}
	}
	return idx.decide(q, scored, opts)
}

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

// scoredProduct is one candidate, what it scored, and what it contradicts.
type scoredProduct struct {
	product *MasterProduct
	score   float64
	// conflicts is everything this candidate disagrees with the row about, and
	// mass is their weight. They rank the candidate BEFORE the score does: a
	// candidate that contradicts nothing the row states comes first, however
	// well a contradicting one is spelled. See discriminate.go.
	conflicts []conflict
	mass      float64
	// evidence is the score BEFORE the contradictions are applied — how alike
	// the two are, before asking whether they can be the same product. It is
	// what the shortlist is filtered on, so a candidate the row contradicts is
	// still shown to the reviewer with the contradiction named, rather than
	// vanishing and leaving "no similar product found" as the only explanation.
	evidence float64
	// name is the name similarity alone, and agreed is which attributes
	// corroborated. They are kept rather than the rendered explanation because
	// only the handful of candidates that reach the review screen need one —
	// see scoredProduct.describeReason.
	name   float64
	agreed agreements
	exact  bool
	// blind means not one distinctive word agreed: the whole likeness comes
	// from a wordless channel — character trigrams, or the cross-script
	// consonant skeleton. See settleable.
	blind bool
}

// settleable reports whether a candidate may be APPLIED rather than merely
// offered.
//
// A blind candidate is one whose entire case is that the letters line up. The
// skeleton channel discards every vowel and folds both alphabets onto one
// alphabet, which is what lets an Arabic row find its Latin catalogue entry —
// and also what let these through, each applied silently at "تشابه الاسم 86%"
// with nothing else to say for itself:
//
//	chromax 60 capsules   ->  سيرومكس 10 اقراص      (want كروماكس 60 كبسولة)
//	alkasilon 200ml       ->  لوكاسالين مرهم 10جم   (want الكازيلون معلق 200 مل)
//	c zinc 30 caps        ->  زنك 50مجم 100 قرص     (want سي زنك 30 كبسولة)
//
// Letters lining up is real evidence and it stays worth what it is worth: the
// row is still scored, still shortlisted, still one click from acceptance. What
// it may no longer do is decide. That is the whole of the difference between a
// suggestion and a silent write into a vendor's catalogue.
//
// Two exceptions, and both are measured rather than assumed:
//
//   - A dose stated on both sides and agreeing. Two products spelled alike by
//     coincidence do not also agree on 500 mg by coincidence.
//   - A name similarity at or above blindNearIdentical. The skeleton channel is
//     capped at 0.86 and cannot reach it, so this is the trigram channel saying
//     the two strings are all but the same string — one transposed letter in a
//     brand somebody typed twice. That is the case this whole channel exists
//     for, and refusing it costs the transliteration matches without buying any
//     precision back.
//
// Without the exceptions the rule removes 8 wrong matches and 124 right ones.
// With them it removes the same 8 and 3 right ones. The exceptions are not a
// softening of the rule; they are what makes it pay.
func (s scoredProduct) settleable() bool {
	return !s.blind || s.agreed&agreedDose != 0 || s.name >= blindNearIdentical
}

// blindNearIdentical is the name similarity at which letters alone are allowed
// to settle a match. See settleable.
const blindNearIdentical = 0.90

// score rates every plausible catalogue product for one query.
func (idx *Index) score(q *query, opts MatchOptions) []scoredProduct {
	pool := idx.candidatePoolWithKeys(q.tokens, q.keys, opts.PoolLimit)
	if q.nameKey != "" {
		pool = append(pool, idx.byName[q.nameKey]...)
	}
	if len(pool) == 0 {
		return nil
	}

	seen := make(map[int64]bool, len(pool))
	out := make([]scoredProduct, 0, len(pool))
	for _, p := range pool {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		if sp, ok := idx.rate(q, p); ok && sp.evidence >= opts.MinReview {
			out = append(out, sp)
		}
	}

	// Contradiction first, similarity second.
	//
	// This ordering is the change that makes "the right size is in the
	// catalogue and the wrong one was chosen" impossible to produce by
	// arithmetic: the wrong size contradicts a figure the row states and the
	// right size does not, so the right size sorts ahead of it even when the
	// wrong one is spelled more like the row.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].mass != out[j].mass {
			return out[i].mass < out[j].mass
		}
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].product.ID < out[j].product.ID
	})
	return out
}
