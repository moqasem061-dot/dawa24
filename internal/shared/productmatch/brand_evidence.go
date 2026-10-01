package productmatch

// What a match must rest on before it is applied without asking.
//
// A row's brand word links to a candidate in one of three ways (see
// identity_brand_guard.go): as written, by sound, or by a typo-sized
// difference. Only the first identifies a product on its own. The other two
// are how a misspelt brand is found — and also how two different Egyptian
// brands that happen to share their consonants are confused:
//
//	تيرافيريا 550مجم 30 قرص   →  تريفيرا زيت للشعر       (a hair oil)
//	حوينتون 10 كيس            →  هاينوتون 10 اكياس        (want جوينتون)
//	رستين 20 اقراص            →  اورسوتوين 20 كبسولة
//
// So a loose link is applied only when something the row STATES — its dose,
// its form, its count, its company, its molecule, its printed price — is also
// what the candidate states.
//
// Stricter versions were measured and rejected: requiring the DOSE in
// particular (rather than any confirmation) refused 112 correct matches for
// every wrong one it prevented on the egypt suite — most loose brand links are
// misspellings of the right product, and a pharmacy line often omits the dose.

// corroborations are the agreements that can confirm a loose brand link.
const corroborations = agreedDose | agreedForm | agreedMaker | agreedMolecule | agreedPrice | agreedCount

// strongCorroborations each confirm a loose brand link alone; the weak ones —
// a form and a count that hundreds of products share — must come in pairs.
const (
	strongCorroborations = agreedDose | agreedMaker | agreedMolecule | agreedPrice
	weakCorroborations   = agreedForm | agreedCount
)

func bitsSet(a agreements) int {
	n := 0
	for a != 0 {
		n += int(a & 1)
		a >>= 1
	}
	return n
}

// evidenced reports whether the candidate may be applied on what it rests on.
func (s scoredProduct) evidenced(q *query) bool {
	if q.brand == "" || s.brandLink == brandLinkExact {
		return true
	}
	if s.agreed&corroborations == 0 {
		return false
	}
	if s.brandLink != brandLinkCross {
		return true
	}
	// Across alphabets one weak confirmation is not enough: measured, this
	// prevents 8 wrong matches for every 150 rows it sends to review on Latin
	// supplier files, where within one alphabet the same rule cost 42 correct
	// matches per wrong one prevented and is not applied.
	if s.agreed&strongCorroborations != 0 {
		return true
	}
	return bitsSet(s.agreed&weakCorroborations) >= 2
}

// brandContestMargin is how close a second candidate carrying the same brand
// word must score before the choice between them is a spelling question.
const brandContestMargin = 0.12

// brandContested reports a best candidate that carries the row's brand only
// loosely while another uncontradicted candidate carries it as well and
// scores close behind. Neither link is proof, so the row is not settled on
// arithmetic.
func brandContested(scored []scoredProduct, limit int) bool {
	if len(scored) < 2 {
		return false
	}
	best := scored[0]
	if best.brandLink == brandLinkExact || best.brandLink == brandLinkNone {
		return false
	}
	for i := 1; i < len(scored) && i < limit; i++ {
		s := scored[i]
		if s.brandLink == brandLinkNone || len(hardOf(s.conflicts)) > 0 {
			continue
		}
		if best.score-s.score < brandContestMargin {
			return true
		}
	}
	return false
}
