package matching

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ProductFactFingerprint produces a deterministic sha256 fingerprint of a catalogue product's
// decisive identity attributes (data-model §3.4, R8).
func ProductFactFingerprint(p *productmatch.MasterProduct) string {
	if p == nil {
		return ""
	}
	h := sha256.New()
	parts := []string{
		p.NameAR,
		p.NameEN,
		p.DosageForm,
		p.Concentration,
		p.Unit,
		p.Manufacturer,
		p.Barcode,
		p.SKU,
	}
	for i, part := range parts {
		if i > 0 {
			h.Write([]byte{0x1f})
		}
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CandidateSetFingerprint computes a stable fingerprint of candidate IDs for negative AI answers.
func CandidateSetFingerprint(candidateIDs []int64) string {
	sorted := make([]int64, len(candidateIDs))
	copy(sorted, candidateIDs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var b strings.Builder
	for i, id := range sorted {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// KnowledgeResolver resolves match questions against scoped knowledge according to precedence,
// staleness detection, and Gate evaluation (US8 T109).
type KnowledgeResolver struct {
	repo KnowledgeRepository
}

// NewKnowledgeResolver creates a KnowledgeResolver.
func NewKnowledgeResolver(repo KnowledgeRepository) *KnowledgeResolver {
	return &KnowledgeResolver{repo: repo}
}

// KnowledgeKindRank assigns numeric precedence for resolution (R8):
// human_reject (5) > human_confirm (4) > org curated (3) > platform_fact (2) > ai_answer (1).
func KnowledgeKindRank(kind string, isOrg bool) int {
	switch kind {
	case "human_reject":
		return 50
	case "human_confirm":
		if isOrg {
			return 45
		}
		return 40
	case "curated_alias", "curated_relationship", "supplier_code":
		if isOrg {
			return 35
		}
		return 30
	case "platform_fact":
		return 20
	case "ai_answer":
		return 10
	default:
		return 0
	}
}

// Resolve looks up knowledge matching the signature and applies precedence, staleness, and Gate rules.
func (kr *KnowledgeResolver) Resolve(
	ctx context.Context,
	sc Scope,
	row *productmatch.Row,
	sig string,
	idx *productmatch.Index,
	pol productmatch.Policy,
	candidateIDs []int64,
) (*matchflow.RowDecision, bool, error) {
	if kr == nil || kr.repo == nil || sig == "" {
		return nil, false, nil
	}

	// 1. Fetch org knowledge (RLS-scoped)
	orgItems, err := kr.repo.LookupOrg(ctx, []string{sig})
	if err != nil {
		return nil, false, err
	}

	// 2. Fetch platform knowledge (database.AsSystem, respecting opt-out)
	platItems, err := kr.repo.LookupPlatform(ctx, []string{sig}, sc.EffectiveOrg)
	if err != nil {
		return nil, false, err
	}

	all := make([]Knowledge, 0, len(orgItems)+len(platItems))
	all = append(all, orgItems...)
	all = append(all, platItems...)

	if len(all) == 0 {
		return nil, false, nil
	}

	// 3. Filter by compatible scope & sort by precedence
	var compatible []Knowledge
	for _, k := range all {
		// Namespace compatibility
		if k.SourceNamespace != "*" && k.SourceNamespace != string(sc.Workflow) && k.SourceNamespace != sc.EntryPoint {
			continue
		}
		compatible = append(compatible, k)
	}

	if len(compatible) == 0 {
		return nil, false, nil
	}

	sort.SliceStable(compatible, func(i, j int) bool {
		rankI := KnowledgeKindRank(compatible[i].Kind, compatible[i].OrganizationID != nil)
		rankJ := KnowledgeKindRank(compatible[j].Kind, compatible[j].OrganizationID != nil)
		if rankI != rankJ {
			return rankI > rankJ
		}
		// Newer entries win ties
		return compatible[i].CreatedAt.After(compatible[j].CreatedAt)
	})

	// 4. Evaluate top applicable entry
	for _, k := range compatible {
		// Rule: AI answer ignored when AI is disabled in policy (FR-057)
		if k.Kind == "ai_answer" && !pol.AI.Enabled {
			continue
		}

		// Rule: Negative AI answer valid only while current candidates are a subset of stored candidates
		if k.Kind == "ai_answer" && (k.TargetProductID == nil || *k.TargetProductID <= 0) {
			if k.CandidateSetFingerprint != nil && *k.CandidateSetFingerprint != "" {
				currentFP := CandidateSetFingerprint(candidateIDs)
				if currentFP != *k.CandidateSetFingerprint {
					continue
				}
			}
		}

		// Staleness check
		isStale := false
		var targetProduct *productmatch.MasterProduct
		if k.TargetProductID != nil && *k.TargetProductID > 0 {
			if idx != nil {
				prod, ok := idx.Lookup(*k.TargetProductID)
				if !ok || prod == nil {
					isStale = true
				} else {
					targetProduct = prod
					if k.TargetFactFingerprint != "" && ProductFactFingerprint(prod) != k.TargetFactFingerprint {
						isStale = true
					}
				}
			}
		}

		// Policy compatibility check
		if len(pol.KnowledgeCompat) > 0 && k.PolicyVersion != "" {
			compat := false
			for _, cv := range pol.KnowledgeCompat {
				if cv == k.PolicyVersion {
					compat = true
					break
				}
			}
			if !compat {
				isStale = true
			}
		}

		// If human_reject
		if k.Kind == "human_reject" {
			dec := matchflow.RowDecision{
				V:        1,
				State:    matchflow.StateNoCandidate,
				Basis:    matchflow.BasisKnowledgeHuman,
				Reasons:  []matchflow.Reason{{Code: "human_reject"}},
				Revision: row.Number,
				Versions: matchflow.Versions{
					Policy:          pol.Version,
					Normalizer:      productmatch.NormalizerVersion,
					CatalogRevision: 1,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateOff},
			}
			return &dec, true, nil
		}

		// If target product is specified, evaluate with Gate
		if targetProduct != nil {
			gate := productmatch.NewGate(idx)
			prop := productmatch.Proposal{
				Path:           productmatch.PathKnowledge,
				ProductID:      targetProduct.ID,
				KnowledgeStale: isStale,
			}
			verdict := gate.Evaluate(productmatch.ExtractRowFacts(row, nil), prop, pol)

			basis := matchflow.BasisKnowledgeHuman
			switch k.Kind {
			case "curated_alias", "curated_relationship", "supplier_code":
				basis = matchflow.BasisKnowledgeCurated
			case "platform_fact":
				basis = matchflow.BasisKnowledgePlatform
			case "ai_answer":
				basis = matchflow.BasisAI
			}

			reasons := make([]matchflow.Reason, len(verdict.Reasons))
			for idxR, vr := range verdict.Reasons {
				reasons[idxR] = matchflow.Reason{Code: string(vr.Code)}
			}

			gran := matchflow.GranularityPair{
				Required: matchflow.Granularity(pol.Required),
				Achieved: matchflow.Granularity(verdict.Achieved),
			}

			dec := matchflow.RowDecision{
				V:           1,
				State:       verdict.State,
				Basis:       basis,
				ProductID:   &targetProduct.ID,
				Granularity: gran,
				Pack: matchflow.PackDecision{
					Outcome:        verdict.Pack,
					RelationshipID: verdict.RelationshipID,
				},
				Reasons:  reasons,
				Revision: row.Number,
				Versions: matchflow.Versions{
					Policy:          pol.Version,
					Normalizer:      productmatch.NormalizerVersion,
					CatalogRevision: 1,
				},
				AI: matchflow.AIDecision{State: matchflow.AIStateOff},
			}

			if isStale {
				dec.Flags = append(dec.Flags, "stale_knowledge")
			}

			return &dec, true, nil
		}
	}

	return nil, false, nil
}
