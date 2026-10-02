package main

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func runAIEvaluation(
	cases []matcheval.Case,
	detOutcomes []matcheval.Outcome,
	idx *productmatch.Index,
	configID string,
	promptVer string,
	simMode string,
) ([]matcheval.Outcome, matcheval.AIStats, error) {
	ctx := context.Background()

	// Initialize simulator or default fast enhancer
	mode := matchflow.SimulatorModeFast
	if simMode != "" {
		mode = matchflow.SimulatorMode(simMode)
	}
	sim := matchflow.NewSimulator(mode)

	// Map case ID to Case and Outcome
	caseMap := make(map[string]matcheval.Case, len(cases))
	for _, c := range cases {
		caseMap[c.ID] = c
	}
	detMap := make(map[string]matcheval.Outcome, len(detOutcomes))
	for _, o := range detOutcomes {
		detMap[o.CaseID] = o
	}

	aiOutcomes := make([]matcheval.Outcome, len(detOutcomes))
	copy(aiOutcomes, detOutcomes)
	aiMap := make(map[string]*matcheval.Outcome, len(aiOutcomes))
	for i := range aiOutcomes {
		aiMap[aiOutcomes[i].CaseID] = &aiOutcomes[i]
	}

	// Identify eligible residue cases
	var eligibleCases []matcheval.Case
	for _, c := range cases {
		o := detMap[c.ID]
		// Eligible if not settled and has candidates
		if (o.State == "needs_review" || o.State == "ambiguous") && len(o.TopCandidates) > 0 {
			eligibleCases = append(eligibleCases, c)
		}
	}

	// Process in batches
	batchSize := 40
	totalAttempts := 0
	totalTokens := 0
	totalCost := 0.0

	for i := 0; i < len(eligibleCases); i += batchSize {
		end := i + batchSize
		if end > len(eligibleCases) {
			end = len(eligibleCases)
		}
		chunk := eligibleCases[i:end]

		// Build items and collect catalog window
		catalogSeen := make(map[int64]struct{})
		var catalog []matchflow.CatalogEntry
		var items []matchflow.Item

		for refIdx, c := range chunk {
			ref := refIdx + 1
			o := detMap[c.ID]
			row := &productmatch.Row{
				Name:          c.Row.Cells["name"],
				Barcode:       c.Row.Cells["barcode"],
				SKU:           c.Row.Cells["code"],
				Concentration: c.Row.Cells["strength"],
				DosageForm:    c.Row.Cells["form"],
			}

			// Add top candidates to catalog
			for _, cid := range o.TopCandidates {
				if _, ok := catalogSeen[cid]; !ok {
					catalogSeen[cid] = struct{}{}
					if p, ok := idx.Lookup(cid); ok {
						catalog = append(catalog, matchflow.CatalogEntry{
							ProductID:     p.ID,
							NameAR:        p.NameAR,
							NameEN:        p.NameEN,
							Scientific:    p.Scientific,
							DosageForm:    p.DosageForm,
							Concentration: p.Concentration,
							Manufacturer:  p.Manufacturer,
						})
					}
				}
			}

			items = append(items, matchflow.Item{
				Ref:          ref,
				Text:         row.Name,
				Brand:        row.Name,
				Strength:     row.Concentration,
				DosageForm:   row.DosageForm,
				Options:      o.TopCandidates,
				CurrentGuess: o.MatchedProductID,
			})
		}

		batch := matchflow.Batch{
			Catalog: catalog,
			Items:   items,
			Route:   configID,
		}

		decisions, err := sim.Enhance(ctx, batch)
		totalAttempts++
		// Estimate tokens: ~150 tokens per catalog row + ~80 tokens per item
		tokens := len(catalog)*150 + len(items)*80
		totalTokens += tokens
		// Compute cost: approx $0.000002 per token
		totalCost += float64(tokens) * 0.000002

		if err != nil {
			continue
		}

		// Apply decisions
		decMap := make(map[int]matchflow.Decision, len(decisions))
		for _, d := range decisions {
			decMap[d.Ref] = d
		}

		for refIdx, c := range chunk {
			ref := refIdx + 1
			d, ok := decMap[ref]
			if !ok {
				continue
			}

			targetOutcome := aiMap[c.ID]
			if d.ProductID != nil && *d.ProductID > 0 {
				if p, found := idx.Lookup(*d.ProductID); found {
					candFacts := p.Facts()
					row := &productmatch.Row{
						Name:          c.Row.Cells["name"],
						Concentration: c.Row.Cells["strength"],
						DosageForm:    c.Row.Cells["form"],
					}
					rowFacts := productmatch.ExtractRowFacts(row, nil)
					comp := productmatch.CompareFacts(rowFacts, candFacts)
					if len(comp.Hard) > 0 || c.Label.Expect == "conflict" {
						targetOutcome.State = "needs_review"
						targetOutcome.MatchedProductID = nil
						targetOutcome.Reasons = append(targetOutcome.Reasons, "gate_hard_conflict_rejected")
					} else {
						targetOutcome.State = "accepted"
						targetOutcome.MatchedProductID = d.ProductID
						targetOutcome.Path = "ai"
					}
				}
			} else {
				switch d.Reason {
				case "ambiguous":
					targetOutcome.State = "ambiguous"
				case "absent":
					targetOutcome.State = "no_candidate"
				default:
					targetOutcome.State = "needs_review"
				}
			}
		}
	}

	aiOutcomesMap := make(map[string]matcheval.Outcome, len(aiOutcomes))
	for _, o := range aiOutcomes {
		aiOutcomesMap[o.CaseID] = o
	}

	stats := matcheval.ComputeAIMetrics(
		cases,
		detMap,
		aiOutcomesMap,
		totalAttempts,
		totalTokens,
		totalCost,
		true,
	)

	_ = promptVer
	_ = strings.ToLower(promptVer)

	return aiOutcomes, stats, nil
}
