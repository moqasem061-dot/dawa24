package matching

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// DiagnosticRow provides structured diagnostics for a single match row (FR-104, US12 T105).
type DiagnosticRow struct {
	RunID            int64                   `json:"run_id"`
	OrganizationID   int64                   `json:"organization_id"`
	Workflow         string                  `json:"workflow"`
	State            string                  `json:"state"`
	Versions         matchflow.Versions      `json:"versions"`
	HeartbeatAt      time.Time               `json:"heartbeat_at"`
	CreatedAt        time.Time               `json:"created_at"`
	OriginalFacts    map[string]string       `json:"original_facts"`
	InferredFacts    map[string]string       `json:"inferred_facts"`
	CorrectedFacts   []matchflow.Correction  `json:"corrected_facts"`
	Decision         matchflow.RowDecision   `json:"decision"`
	BaselineDecision *matchflow.RowDecision  `json:"baseline_decision,omitempty"`
	Candidates       []CandidateDiagnostic   `json:"candidates"`
	NeverRetrieved   []NeverRetrievedProduct `json:"never_retrieved"`
	RevisionHistory  []RevisionEntry         `json:"revision_history"`
	AIAttempts       []AIAttemptInfo         `json:"ai_attempts"`
}

// CandidateDiagnostic captures candidate evaluation reasons and differences.
type CandidateDiagnostic struct {
	ProductID int64             `json:"product_id"`
	NameAR    string            `json:"name_ar"`
	NameEN    string            `json:"name_en"`
	Score     float64           `json:"score"`
	Eligible  bool              `json:"eligible"`
	Reasons   []string          `json:"reasons"`
	Diff      map[string]string `json:"diff"`
}

// NeverRetrievedProduct documents targets not found in the initial retrieval recall.
type NeverRetrievedProduct struct {
	ProductID int64  `json:"product_id"`
	NameAR    string `json:"name_ar"`
	NameEN    string `json:"name_en"`
	Reason    string `json:"reason"`
}

// RevisionEntry captures an audit snapshot of a revision step.
type RevisionEntry struct {
	Revision int       `json:"revision"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Action   string    `json:"action"`
}

// AIAttemptInfo documents an AI processing attempt for this question.
type AIAttemptInfo struct {
	AttemptNo int       `json:"attempt_no"`
	State     string    `json:"state"`
	Outcome   string    `json:"outcome"`
	At        time.Time `json:"at"`
}

// BuildDiagnosticRow constructs detailed diagnostics for a row under a run (FR-104).
func BuildDiagnosticRow(
	idx *productmatch.Index,
	run *MatchRun,
	row productmatch.Row,
	dec matchflow.RowDecision,
) DiagnosticRow {
	diag := DiagnosticRow{
		RunID:          run.ID,
		OrganizationID: run.OrganizationID,
		Workflow:       run.Workflow,
		State:          run.State,
		Versions:       dec.Versions,
		HeartbeatAt:    run.HeartbeatAt,
		CreatedAt:      run.CreatedAt,
		OriginalFacts:  make(map[string]string),
		InferredFacts:  make(map[string]string),
		Decision:       dec,
	}

	// 1. Original facts from source row
	diag.OriginalFacts["name"] = strings.TrimSpace(row.DisplayName())
	if row.SKU != "" {
		diag.OriginalFacts["sku"] = row.SKU
	}
	if row.Barcode != "" {
		diag.OriginalFacts["barcode"] = row.Barcode
	}
	if row.DosageForm != "" {
		diag.OriginalFacts["dosage_form"] = row.DosageForm
	}
	if row.Concentration != "" {
		diag.OriginalFacts["concentration"] = row.Concentration
	}
	if row.Manufacturer != "" {
		diag.OriginalFacts["manufacturer"] = row.Manufacturer
	}
	if row.Unit != "" {
		diag.OriginalFacts["unit"] = row.Unit
	}

	// 2. Inferred facts from analysis
	norm := productmatch.NormalizeText(row.DisplayName())
	if norm != "" {
		diag.InferredFacts["normalized"] = norm
	}
	stem := productmatch.ShortBrandKey(row.DisplayName())
	if stem != "" {
		diag.InferredFacts["brand_stem"] = stem
	}
	dose := productmatch.FoldDoseText(row.DisplayName())
	if dose != "" {
		diag.InferredFacts["dose_text"] = dose
	}
	skel := productmatch.Skeleton(norm)
	if skel != "" {
		diag.InferredFacts["skeleton"] = skel
	}

	// 3. Corrected facts
	if dec.Actor != nil && len(dec.Actor.Corrections) > 0 {
		diag.CorrectedFacts = dec.Actor.Corrections
	}

	// 4. Candidates (retrieved-but-rejected reasons)
	retrievedIDs := make(map[int64]bool)
	for _, c := range dec.Candidates {
		retrievedIDs[c.ProductID] = true
		cd := CandidateDiagnostic{
			ProductID: c.ProductID,
			Eligible:  c.Eligible,
			Reasons:   c.Reasons,
			Diff:      c.Diff,
		}
		if idx != nil {
			if p, ok := idx.Lookup(c.ProductID); ok && p != nil {
				cd.NameAR = p.NameAR
				cd.NameEN = p.NameEN
			}
		}
		diag.Candidates = append(diag.Candidates, cd)
	}

	// 5. Never-retrieved products: check if any close matches exist in the index that weren't retrieved
	if idx != nil && len(diag.Candidates) < 5 {
		recalled := idx.Recall(&row, productmatch.DefaultRecallOptions())
		for _, rc := range recalled {
			if !retrievedIDs[rc.ProductID] {
				nameAR := rc.Name
				nameEN := ""
				if p, ok := idx.Lookup(rc.ProductID); ok && p != nil {
					nameAR = p.NameAR
					nameEN = p.NameEN
				}
				diag.NeverRetrieved = append(diag.NeverRetrieved, NeverRetrievedProduct{
					ProductID: rc.ProductID,
					NameAR:    nameAR,
					NameEN:    nameEN,
					Reason:    "خارج نطاق الاسترجاع الأولي (Not retrieved by recall filter)",
				})
				if len(diag.NeverRetrieved) >= 5 {
					break
				}
			}
		}
	}

	// 6. Revision history
	diag.RevisionHistory = append(diag.RevisionHistory, RevisionEntry{
		Revision: run.SourceRevision,
		Actor:    run.CreatedBy,
		At:       run.CreatedAt,
		Action:   "initial_preparation",
	})
	if dec.Revision > run.SourceRevision && dec.Actor != nil {
		diag.RevisionHistory = append(diag.RevisionHistory, RevisionEntry{
			Revision: dec.Revision,
			Actor:    dec.Actor.UserID,
			At:       dec.Actor.At,
			Action:   "human_review_correction",
		})
	}

	// 7. AI attempts telemetry
	if dec.AI.State != "" && dec.AI.State != matchflow.AIStateOff {
		diag.AIAttempts = append(diag.AIAttempts, AIAttemptInfo{
			AttemptNo: len(dec.AI.Attempts),
			State:     string(dec.AI.State),
			Outcome:   dec.AI.Outcome,
			At:        run.HeartbeatAt,
		})
	}

	return diag
}

// Diagnose evaluates and returns the full diagnostic view for a row in a run (US12 T105).
// It strictly enforces tenant isolation: non-matching organization IDs are rejected.
func (s *serviceImpl) Diagnose(
	ctx context.Context,
	sc Scope,
	runID int64,
	row productmatch.Row,
	dec matchflow.RowDecision,
) (DiagnosticRow, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return DiagnosticRow{}, fmt.Errorf("diagnose: run %d: %w", runID, err)
	}

	// Tenant boundary check (Rule 4 & FR-096):
	// A tenant user cannot inspect another tenant's run diagnostics.
	if sc.EffectiveOrg > 0 && run.OrganizationID != sc.EffectiveOrg {
		return DiagnosticRow{}, fmt.Errorf("diagnose: forbidden: run belongs to org %d", run.OrganizationID)
	}

	idx, err := s.indexCache.Get(ctx, "master", run.CatalogRevision)
	if err != nil || idx == nil {
		idx = productmatch.NewIndex(nil)
	}

	return BuildDiagnosticRow(idx, run, row, dec), nil
}
