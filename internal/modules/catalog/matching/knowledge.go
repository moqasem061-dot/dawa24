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

// Resolution is what scoped knowledge says about one question.
type Resolution struct {
	// Decision is the highest-precedence remembered answer, evaluated by the
	// gate against the current catalogue. Nil when no entry applies.
	Decision *matchflow.RowDecision
	// Accepted says Decision passed the gate and may be applied as it stands.
	// A stale or now-contradicted memory is returned unaccepted: the caller
	// re-decides, and keeps the memory only as a suggestion.
	Accepted bool
	// Rejected are the products a person rejected for this question. They are
	// vetoed, for this question only, whatever else proposes them.
	Rejected []int64
	// RejectAll says a person stated that no catalogue product is this row.
	RejectAll bool
}

// knowledgeFetchChunk bounds one lookup query.
const knowledgeFetchChunk = 500

// Fetch loads every applicable knowledge entry for a set of questions in a
// few queries, rather than two per question.
func (kr *KnowledgeResolver) Fetch(ctx context.Context, sc Scope, sigs []string) (map[string][]Knowledge, error) {
	out := make(map[string][]Knowledge, len(sigs))
	if kr == nil || kr.repo == nil || len(sigs) == 0 {
		return out, nil
	}
	for start := 0; start < len(sigs); start += knowledgeFetchChunk {
		chunk := sigs[start:min(start+knowledgeFetchChunk, len(sigs))]
		org, err := kr.repo.LookupOrg(ctx, chunk)
		if err != nil {
			return out, err
		}
		plat, err := kr.repo.LookupPlatform(ctx, chunk, sc.EffectiveOrg)
		if err != nil {
			return out, err
		}
		for _, k := range append(org, plat...) {
			out[k.QuestionSignature] = append(out[k.QuestionSignature], k)
		}
	}
	return out, nil
}

// Resolve looks up and evaluates the knowledge for one question. Kept for
// callers that ask about a single row; Prepare uses Fetch and Evaluate.
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
	items, err := kr.Fetch(ctx, sc, []string{sig})
	if err != nil {
		return nil, false, err
	}
	res := kr.Evaluate(sc, row, items[sig], idx, pol, candidateIDs)
	switch {
	case res.RejectAll:
		dec := rejectAllDecision(pol)
		return &dec, true, nil
	case res.Decision != nil:
		return res.Decision, true, nil
	}
	return nil, false, nil
}

func rejectAllDecision(pol productmatch.Policy) matchflow.RowDecision {
	return matchflow.RowDecision{
		V:        1,
		State:    matchflow.StateNoCandidate,
		Basis:    matchflow.BasisKnowledgeHuman,
		Reasons:  []matchflow.Reason{{Code: string(productmatch.ReasonKnowledgeRejected)}},
		Versions: matchflow.Versions{Policy: pol.Version, Normalizer: productmatch.NormalizerVersion},
		AI:       matchflow.AIDecision{State: matchflow.AIStateOff},
	}
}

// Evaluate applies scope, precedence, staleness and the gate to the entries
// remembered for one question.
func (kr *KnowledgeResolver) Evaluate(
	sc Scope,
	row *productmatch.Row,
	items []Knowledge,
	idx *productmatch.Index,
	pol productmatch.Policy,
	candidateIDs []int64,
) Resolution {
	var res Resolution
	var compatible []Knowledge
	for _, k := range items {
		if k.SourceNamespace != "*" && k.SourceNamespace != string(sc.Workflow) && k.SourceNamespace != sc.EntryPoint {
			continue
		}
		compatible = append(compatible, k)
	}
	if len(compatible) == 0 {
		return res
	}
	sort.SliceStable(compatible, func(i, j int) bool {
		rankI := KnowledgeKindRank(compatible[i].Kind, compatible[i].OrganizationID != nil)
		rankJ := KnowledgeKindRank(compatible[j].Kind, compatible[j].OrganizationID != nil)
		if rankI != rankJ {
			return rankI > rankJ
		}
		return compatible[i].CreatedAt.After(compatible[j].CreatedAt)
	})

	// Rejections first. A rejection naming a product vetoes that product; one
	// naming none says no catalogue product is this row, unless a person has
	// confirmed a product for this question more recently.
	var newestConfirm, newestRejectAll *Knowledge
	for i := range compatible {
		k := &compatible[i]
		switch {
		case k.Kind == "human_reject" && k.TargetProductID != nil && *k.TargetProductID > 0:
			res.Rejected = append(res.Rejected, *k.TargetProductID)
		case k.Kind == "human_reject":
			if newestRejectAll == nil || k.CreatedAt.After(newestRejectAll.CreatedAt) {
				newestRejectAll = k
			}
		case k.Kind == "human_confirm":
			if newestConfirm == nil || k.CreatedAt.After(newestConfirm.CreatedAt) {
				newestConfirm = k
			}
		}
	}
	if newestRejectAll != nil && (newestConfirm == nil || newestRejectAll.CreatedAt.After(newestConfirm.CreatedAt)) {
		res.RejectAll = true
		return res
	}

	for _, k := range compatible {
		if k.Kind == "human_reject" {
			continue
		}
		if k.Kind == "ai_answer" && !pol.AI.Enabled {
			continue // FR-057: AI-only memory has no effect with AI off
		}
		if k.TargetProductID == nil || *k.TargetProductID <= 0 {
			continue // a remembered "none of these" from a model is not applied
		}
		if containsID(res.Rejected, *k.TargetProductID) || idx == nil {
			continue
		}
		target, ok := idx.Lookup(*k.TargetProductID)
		if !ok || target == nil {
			continue // the product is gone; nothing to apply or suggest
		}

		stale := k.TargetFactFingerprint != "" && ProductFactFingerprint(target) != k.TargetFactFingerprint
		if len(pol.KnowledgeCompat) > 0 && k.PolicyVersion != "" && !compatVersion(pol.KnowledgeCompat, k.PolicyVersion) {
			stale = true
		}

		verdict := productmatch.NewGate(idx).Evaluate(productmatch.ExtractRowFacts(row, nil), productmatch.Proposal{
			Path:           productmatch.PathKnowledge,
			ProductID:      target.ID,
			KnowledgeStale: stale,
			Row:            row,
		}, pol)

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
		for i, vr := range verdict.Reasons {
			reasons[i] = matchflow.Reason{Code: string(vr.Code)}
		}
		achieved := verdict.Achieved
		if achieved == productmatch.GranularityNone && verdict.State == matchflow.StateAccepted {
			achieved = productmatch.GranularityProduct
		}
		id := target.ID
		dec := matchflow.RowDecision{
			V:         1,
			State:     verdict.State,
			Basis:     basis,
			ProductID: &id,
			Granularity: matchflow.GranularityPair{
				Required: pol.Required.MatchflowGranularity(),
				Achieved: achieved.MatchflowGranularity(),
			},
			Pack:    matchflow.PackDecision{Outcome: verdict.Pack, RelationshipID: verdict.RelationshipID},
			Reasons: reasons,
			Versions: matchflow.Versions{
				Policy:     pol.Version,
				Normalizer: productmatch.NormalizerVersion,
				Target:     ProductFactFingerprint(target),
			},
			AI: matchflow.AIDecision{State: matchflow.AIStateOff},
		}
		if stale {
			dec.Flags = append(dec.Flags, "stale_knowledge")
		}
		res.Decision = &dec
		res.Accepted = verdict.State == matchflow.StateAccepted
		return res
	}
	return res
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func compatVersion(allowed []string, v string) bool {
	for _, a := range allowed {
		if a == v || a == "*" {
			return true
		}
	}
	return false
}
