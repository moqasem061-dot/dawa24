package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// Path identifies which strategy generated the match proposal.
type Path uint8

const (
	PathIdentifier Path = iota
	PathKnowledge
	PathExact
	PathSimilarity
	PathReviewer
	PathAI
)

// AIEvidence holds per-attribute verification and qualification status from AI.
type AIEvidence struct {
	Attributes map[string]Outcome `json:"attributes,omitempty"`
	Disagrees  bool               `json:"disagrees,omitempty"`
	Qualified  bool               `json:"qualified,omitempty"`
}

// Proposal represents a candidate match submitted to the Gate for acceptance.
type Proposal struct {
	Path             Path
	ProductID        int64
	Score            float64
	Siblings         []int64
	Evidence         *AIEvidence
	LettersOnly      bool
	Namespace        string
	IdentifierValid  bool
	IdentifierUnique bool
	KnowledgeStale   bool
}

// PackOutcome indicates the result of comparing pack units and quantities.
type PackOutcome string

const (
	PackOutcomeAgree             PackOutcome = "agree"
	PackOutcomeConflict          PackOutcome = "conflict"
	PackOutcomeUnstated          PackOutcome = "unstated"
	PackOutcomeNeedsRelationship PackOutcome = "needs_relationship"
)

// Verdict is the definitive acceptance decision returned by the Gate.
type Verdict struct {
	Eligible bool
	State    matchflow.State
	Achieved Granularity
	Pack     PackOutcome
	Hard     []ConflictKind
	Reasons  []Reason
}

// ProductLookup retrieves catalogue products by ID.
type ProductLookup interface {
	Lookup(id int64) (*MasterProduct, bool)
}

// Gate is the single authority that evaluates candidates before acceptance.
type Gate struct {
	Catalog    ProductLookup
	PackBridge func(from, to string) (int, bool)
}

// NewGate builds a Gate backed by the given catalog index.
func NewGate(catalog ProductLookup) *Gate {
	return &Gate{Catalog: catalog}
}

// Evaluate applies Gate rules 1–8 in order to determine acceptance.
func (g *Gate) Evaluate(rowFacts IdentityFacts, p Proposal, pol Policy) Verdict {
	// Rule 1: Existence and eligibility
	prod, ok := g.Catalog.Lookup(p.ProductID)
	if !ok || prod == nil {
		return Verdict{
			Eligible: false,
			State:    matchflow.StateNoCandidate,
			Reasons:  []Reason{{Code: ReasonNoCandidate}},
		}
	}

	prodFacts := prod.Facts()
	comp := CompareFacts(rowFacts, prodFacts)

	// Rule 2: Hard conflicts
	if len(comp.Hard) > 0 {
		reasons := make([]Reason, 0, len(comp.Hard)+1)
		for _, h := range comp.Hard {
			reasons = append(reasons, Reason{Code: conflictToReasonCode(h)})
		}
		if p.Path == PathIdentifier {
			reasons = append(reasons, Reason{Code: ReasonIdentifierVsDescription})
		}
		return Verdict{
			Eligible: false,
			State:    matchflow.StateNeedsReview,
			Hard:     comp.Hard,
			Reasons:  reasons,
		}
	}

	// Rule 3: Identifier path extras
	if p.Path == PathIdentifier {
		if !p.IdentifierValid {
			return Verdict{
				Eligible: false,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonIdentifierInvalid}},
			}
		}
		if !p.IdentifierUnique {
			return Verdict{
				Eligible: false,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonIdentifierDuplicate}},
			}
		}
	}

	// Rule 4: Uniqueness / Ambiguity across eligible siblings
	if len(p.Siblings) > 0 {
		for _, sibID := range p.Siblings {
			if sibID == p.ProductID {
				continue
			}
			sib, sibOk := g.Catalog.Lookup(sibID)
			if !sibOk || sib == nil {
				continue
			}
			sibFacts := sib.Facts()
			sibComp := CompareFacts(rowFacts, sibFacts)
			if len(sibComp.Hard) == 0 {
				// Sibling is also eligible and indistinguishable on row's stated facts
				return Verdict{
					Eligible: true,
					State:    matchflow.StateAmbiguous,
					Reasons:  []Reason{{Code: ReasonAmbiguousSiblings}},
				}
			}
		}
	}

	// Rule 5: Omitted attributes
	isUnstated := comp.Attributes["strength"] == OutcomeUnstated ||
		comp.Attributes["form"] == OutcomeUnstated ||
		comp.Attributes["release"] == OutcomeUnstated
	if isUnstated && !pol.AllowOmittedAttrs {
		return Verdict{
			Eligible: true,
			State:    matchflow.StateNeedsReview,
			Reasons:  []Reason{{Code: ReasonAttributeUnstated}},
		}
	}

	// Rule 6: Blind similarity (letters only cannot settle)
	if p.Path == PathSimilarity && p.LettersOnly {
		hasAgreedDose := comp.Attributes["strength"] == OutcomeAgree
		if !hasAgreedDose && p.Score < pol.BlindNearIdentical {
			return Verdict{
				Eligible: true,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonSimilarityLettersOnly}},
			}
		}
	}

	// Rule 7: Threshold and Path qualification
	switch p.Path {
	case PathSimilarity:
		if p.Score < pol.MinStrong {
			return Verdict{
				Eligible: true,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonSimilarity}},
			}
		}
	case PathAI:
		if p.Evidence == nil || !p.Evidence.Qualified {
			return Verdict{
				Eligible: false,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonAIRejectedUnqualified}},
			}
		}
		if p.Evidence.Disagrees {
			return Verdict{
				Eligible: false,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonAIDisagrees}},
			}
		}
	case PathKnowledge:
		if p.KnowledgeStale {
			return Verdict{
				Eligible: false,
				State:    matchflow.StateNeedsReview,
				Reasons:  []Reason{{Code: ReasonKnowledgeStale}},
			}
		}
	case PathReviewer, PathExact, PathIdentifier:
		// No threshold checks
	}

	// Rule 8: Granularity
	achieved := GranularityProduct
	packOutcome := PackOutcomeUnstated

	if comp.Attributes["pack"] == OutcomeAgree {
		achieved = GranularityPack
		packOutcome = PackOutcomeAgree
	} else if comp.Attributes["pack"] == OutcomeConflict {
		packOutcome = PackOutcomeConflict
		if g.PackBridge != nil && rowFacts.Pack != nil && prodFacts.Pack != nil {
			if factor, bOk := g.PackBridge(rowFacts.Pack.SubUnit, prodFacts.Pack.SubUnit); bOk && factor > 0 {
				achieved = GranularityPack
				packOutcome = PackOutcomeAgree
			}
		}
	}

	if pol.Required == GranularityPack && achieved < GranularityPack {
		reasonCode := ReasonPackUnresolved
		if packOutcome == PackOutcomeConflict {
			reasonCode = ReasonPackConflict
		}
		return Verdict{
			Eligible: true,
			State:    matchflow.StateNeedsReview,
			Achieved: achieved,
			Pack:     packOutcome,
			Reasons:  []Reason{{Code: reasonCode}},
		}
	}

	// All 8 rules passed!
	finalState := matchflow.StateAccepted
	if p.Path == PathReviewer {
		finalState = matchflow.StateConfirmed
	}

	return Verdict{
		Eligible: true,
		State:    finalState,
		Achieved: achieved,
		Pack:     packOutcome,
		Reasons:  defaultReasonsForPath(p.Path),
	}
}

func defaultReasonsForPath(path Path) []Reason {
	switch path {
	case PathIdentifier:
		return []Reason{{Code: ReasonIdentifierGTIN}}
	case PathExact:
		return []Reason{{Code: ReasonExactName}}
	case PathSimilarity:
		return []Reason{{Code: ReasonSimilarity}}
	case PathKnowledge:
		return []Reason{{Code: ReasonKnowledgeHuman}}
	case PathAI:
		return []Reason{{Code: ReasonAISelected}}
	case PathReviewer:
		return []Reason{{Code: ReasonKnowledgeHuman}}
	default:
		return nil
	}
}

func conflictToReasonCode(k ConflictKind) ReasonCode {
	switch k {
	case ConflictStrength:
		return ReasonStrengthConflict
	case ConflictStrengthComponent:
		return ReasonComponentConflict
	case ConflictCombination:
		return ReasonCombinationConflict
	case ConflictModifier:
		return ReasonModifierConflict
	case ConflictForm:
		return ReasonFormConflict
	case ConflictRoute:
		return ReasonRouteConflict
	case ConflictRelease:
		return ReasonReleaseConflict
	case ConflictLetter:
		return ReasonLetterConflict
	case ConflictConcentration:
		return ReasonConcentrationConflict
	case ConflictDimension:
		return ReasonDimensionConflict
	case ConflictBrand:
		return ReasonBrandConflict
	case ConflictIdentifierVsDescription:
		return ReasonIdentifierVsDescription
	default:
		return ReasonCode(string(k) + "_conflict")
	}
}
