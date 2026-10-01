package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// Decide evaluates a row against the catalogue under a policy.
//
// Two layers, and the division is deliberate:
//
//   - The decision about WHICH product, and whether arithmetic alone may make
//     it, is the scorer's: retrieval, contradiction-first ranking, and decide()
//     with its measured tie margin and its separated() test. That logic was
//     tuned on live Egyptian files and it is not re-derived here.
//   - Whether that answer may be APPLIED is the gate's: every path — an
//     identifier, remembered knowledge, a reviewer, a model, or the scorer —
//     must pass the same hard-conflict check and the policy's granularity
//     rule before a row is accepted.
//
// The hard-conflict check reads the scorer's own conflict model
// (hard_conflicts.go), so the two layers cannot disagree about whether a
// candidate is the same product.
func (idx *Index) Decide(row *Row, pol Policy) matchflow.RowDecision {
	opts := MatchOptions{
		MinStrong:     pol.MinStrong,
		MinReview:     pol.MinReview,
		PoolLimit:     400,
		MaxCandidates: 5,
	}
	dec, _ := idx.decideRow(row, pol, opts)
	return dec
}

func (idx *Index) decideRow(row *Row, pol Policy, opts MatchOptions) (matchflow.RowDecision, []scoredProduct) {
	catalogRev, normalizerVer := idx.Revision()
	versions := matchflow.Versions{Policy: pol.Version, Normalizer: normalizerVer, CatalogRevision: catalogRev}

	if idx == nil || idx.total == 0 {
		return emptyDecision(matchflow.StateUnavailable, ReasonCatalogUnavailable, versions), nil
	}
	if opts.PoolLimit <= 0 {
		opts.PoolLimit = 400
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 5
	}
	if opts.MinStrong <= 0 {
		opts.MinStrong = DefaultMinStrong
	}
	if opts.MinReview <= 0 {
		opts.MinReview = DefaultMinReview
	}

	q := idx.newQuery(row)
	rowFacts := ExtractRowFacts(row, nil)
	gate := NewGate(idx)

	if dec, ok := idx.decideByIdentifier(row, q, rowFacts, gate, pol, versions); ok {
		return dec, nil
	}

	scored := idx.score(q, opts)
	if len(scored) == 0 {
		return emptyDecision(matchflow.StateNoCandidate, ReasonNoCandidate, versions), nil
	}

	res := idx.decide(q, scored, opts)
	best := scored[0]
	cands := candidateDecisions(scored, opts.MaxCandidates)
	var flags []string
	if q.poolTruncated && !res.Level.Settled() {
		flags = append(flags, "retrieval_incomplete")
	}
	if res.Level.Settled() && q.brand != "" && best.brandLink != brandLinkExact && best.brandLink != brandLinkNone {
		flags = append(flags, FlagLooseBrand)
	}

	var verdict Verdict
	basis := matchflow.BasisSimilarity
	switch res.Level {
	case MatchExact, MatchStrong:
		path := PathSimilarity
		if res.Level == MatchExact {
			path, basis = PathExact, matchflow.BasisExact
		}
		verdict = gate.Evaluate(rowFacts, Proposal{
			Path:      path,
			ProductID: best.product.ID,
			Score:     best.score,
			Hard:      hardOf(best.conflicts),
			HardKnown: true,
			PackClash: hasPackConflict(best.conflicts),
		}, pol)
	case MatchAmbiguous:
		verdict = Verdict{Eligible: true, State: matchflow.StateAmbiguous,
			Reasons: []Reason{{Code: ReasonAmbiguousSiblings}}}
	case MatchReview:
		verdict = Verdict{Eligible: len(hardOf(best.conflicts)) == 0, State: matchflow.StateNeedsReview,
			Hard: hardOf(best.conflicts), Reasons: reviewReasons(q, best)}
	default:
		// Retrieval found products, and every one of them contradicts the row.
		// They stay on the shortlist so the reviewer sees why.
		verdict = Verdict{State: matchflow.StateNoCandidate,
			Hard: hardOf(best.conflicts), Reasons: reviewReasons(q, best)}
	}
	return makeRowDecision(best.product.ID, basis, verdict, versions, pol, cands, flags), scored
}

// decideByIdentifier settles a row on a barcode or a code the user declared,
// when — and only when — the identifier is valid, unique, and the product it
// names does not contradict what the row says about itself.
func (idx *Index) decideByIdentifier(row *Row, q *query, rowFacts IdentityFacts, gate *Gate,
	pol Policy, versions matchflow.Versions) (matchflow.RowDecision, bool) {

	if pol.Identifiers.AllowGTIN && row.Barcode != "" {
		code := sheet.DigitsOnly(row.Barcode)
		// A value that fails the GTIN check digit is a file's own numbering
		// mapped into the barcode column, not a barcode, and it has no authority.
		if ValidGTIN(code) {
			if hits := idx.byBarcode[code]; len(hits) == 1 {
				p := hits[0]
				if dec, ok := idx.identifierDecision(p, q, rowFacts, gate, pol, versions, NamespaceGTIN); ok {
					return dec, true
				}
			}
		}
	}

	if (pol.Identifiers.AllowCatalogCode || pol.Identifiers.AllowSupplierCode) && row.SKU != "" {
		key := sheet.NormalizeCode(row.SKU)
		hits := idx.bySKU[key]
		if len([]rune(key)) < 4 || len(hits) != 1 {
			return matchflow.RowDecision{}, false
		}
		p := hits[0]
		ns := NamespaceCatalogCode
		if !pol.Identifiers.AllowCatalogCode {
			// A supplier's own number colliding with a catalogue code is common
			// and proves nothing on its own. It is used only when the name
			// corroborates it and nothing about the brand disagrees.
			ns = NamespaceSupplierCode
			if len(q.tokens) > 0 && idx.nameSimilarity(q, p) < 0.35 {
				return matchflow.RowDecision{}, false
			}
			for _, c := range idx.conflictsOf(q, p) {
				if c.kind == "brand_unverified" {
					return matchflow.RowDecision{}, false
				}
			}
		}
		return idx.identifierDecision(p, q, rowFacts, gate, pol, versions, ns)
	}
	return matchflow.RowDecision{}, false
}

func (idx *Index) identifierDecision(p *MasterProduct, q *query, rowFacts IdentityFacts, gate *Gate,
	pol Policy, versions matchflow.Versions, ns Namespace) (matchflow.RowDecision, bool) {

	cs := idx.conflictsOf(q, p)
	verdict := gate.Evaluate(rowFacts, Proposal{
		Path:             PathIdentifier,
		ProductID:        p.ID,
		Namespace:        string(ns),
		IdentifierValid:  true,
		IdentifierUnique: true,
		Hard:             hardOf(cs),
		HardKnown:        true,
		PackClash:        hasPackConflict(cs),
	}, pol)
	if verdict.State == matchflow.StateAccepted || len(verdict.Hard) > 0 {
		// Accepted, or an identifier that contradicts its own description —
		// which a person must see, and which scoring must not paper over.
		return makeRowDecision(p.ID, matchflow.BasisIdentifier, verdict, versions, pol, nil, nil), true
	}
	return matchflow.RowDecision{}, false
}

// candidateDecisions renders the shortlist with each candidate's differences.
func candidateDecisions(scored []scoredProduct, limit int) []matchflow.CandidateDecision {
	n := min(len(scored), limit)
	out := make([]matchflow.CandidateDecision, 0, n)
	for i := 0; i < n; i++ {
		s := scored[i]
		diff, reasons := candidateDiff(s.conflicts)
		out = append(out, matchflow.CandidateDecision{
			ProductID: s.product.ID,
			Rank:      i + 1,
			Score:     s.score,
			Eligible:  len(hardOf(s.conflicts)) == 0,
			Diff:      diff,
			Reasons:   reasons,
		})
	}
	return out
}

// reviewReasons says why the best candidate was offered rather than applied.
func reviewReasons(q *query, best scoredProduct) []Reason {
	var out []Reason
	for _, k := range hardOf(best.conflicts) {
		out = append(out, Reason{Code: conflictToReasonCode(k)})
	}
	for _, c := range best.conflicts {
		switch c.kind {
		case "brand_unverified":
			out = append(out, Reason{Code: ReasonBrandUnverified})
		case "count", "pack", "volume":
			out = append(out, Reason{Code: ReasonPackConflict})
		case "modifier_unstated":
			out = append(out, Reason{Code: ReasonModifierUnstated})
		case "letter_unstated":
			out = append(out, Reason{Code: ReasonLetterUnstated})
		case "container":
			out = append(out, Reason{Code: ReasonContainerConflict})
		case "word_unverified":
			out = append(out, Reason{Code: ReasonWordUnverified})
		case "word_mismatch":
			out = append(out, Reason{Code: ReasonWordMismatch})
		case "maker":
			out = append(out, Reason{Code: ReasonMakerConflict})
		case "figure":
			out = append(out, Reason{Code: ReasonFigureConflict})
		}
	}
	if q != nil && !best.evidenced(q) {
		out = append(out, Reason{Code: ReasonBrandUncorroborated})
	}
	if best.blind && !best.settleable() {
		out = append(out, Reason{Code: ReasonSimilarityLettersOnly})
	}
	if len(out) == 0 {
		out = append(out, Reason{Code: ReasonSimilarity})
	}
	return out
}

func emptyDecision(state matchflow.State, code ReasonCode, versions matchflow.Versions) matchflow.RowDecision {
	return matchflow.RowDecision{
		V:        1,
		State:    state,
		Basis:    matchflow.BasisSimilarity,
		Reasons:  []matchflow.Reason{{Code: string(code)}},
		Versions: versions,
		Revision: 1,
		AI:       matchflow.AIDecision{State: matchflow.AIStateOff},
	}
}

func makeRowDecision(
	productID int64,
	basis matchflow.Basis,
	verdict Verdict,
	versions matchflow.Versions,
	pol Policy,
	candidates []matchflow.CandidateDecision,
	flags []string,
) matchflow.RowDecision {
	var prodPtr *int64
	if verdict.State != matchflow.StateNoCandidate &&
		verdict.State != matchflow.StateUnavailable &&
		verdict.State != matchflow.StateInvalidInput &&
		verdict.State != matchflow.StateExcluded &&
		productID > 0 {
		id := productID
		prodPtr = &id
	}

	reasons := make([]matchflow.Reason, 0, len(verdict.Reasons))
	for _, r := range verdict.Reasons {
		reasons = append(reasons, matchflow.Reason{Code: string(r.Code), Params: r.Params})
	}

	achieved := verdict.Achieved
	if achieved == GranularityNone && (verdict.State == matchflow.StateAccepted || verdict.State == matchflow.StateConfirmed) {
		achieved = GranularityProduct
	}
	return matchflow.RowDecision{
		V:         1,
		State:     verdict.State,
		Basis:     basis,
		ProductID: prodPtr,
		Granularity: matchflow.GranularityPair{
			Required: pol.Required.MatchflowGranularity(),
			Achieved: achieved.MatchflowGranularity(),
		},
		Pack:       matchflow.PackDecision{Outcome: matchflow.PackOutcome(verdict.Pack)},
		Candidates: candidates,
		Reasons:    reasons,
		Flags:      flags,
		Versions:   versions,
		Revision:   1,
		AI:         matchflow.AIDecision{State: matchflow.AIStateOff},
	}
}

func conflictKindsToStrings(conflicts []ConflictKind) []string {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]string, len(conflicts))
	for i, c := range conflicts {
		out[i] = string(c) + "_conflict"
	}
	return out
}

// FlagLooseBrand marks an accepted decision whose brand link is not exact.
const FlagLooseBrand = "loose_brand"

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}
