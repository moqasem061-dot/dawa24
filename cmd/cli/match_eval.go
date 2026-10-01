package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func runMatchEval(args []string) int {
	fs := flag.NewFlagSet("match-eval", flag.ContinueOnError)

	corpusDir := fs.String("corpus", "test/corpus", "Path to corpus directory")
	split := fs.String("split", "tuning", "Corpus split (tuning|heldout|adversarial)")
	engine := fs.String("engine", "current", "Engine to evaluate (current|baseline)")
	policy := fs.String("policy", "", "Optional policy file or version")
	ai := fs.Bool("ai", false, "Run AI enhancement stage")
	configID := fs.String("config", "pm-default-v8", "AI configuration ID")
	promptVer := fs.String("prompt", "v8", "Prompt version (v7|v8)")
	aiSim := fs.String("ai-sim", "", "Simulator mode (fast|slow|down|budget|malformed|duplicate_refs|not_offered)")
	budget := fs.String("budget", "", "AI budget file")
	outPath := fs.String("out", "", "Output path for JSON report (required)")
	mdPath := fs.String("md", "", "Output path for Markdown report (optional)")

	_ = budget
	_ = policy

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: parse flags: %v\n", err)
		return 1
	}

	if *outPath == "" {
		fmt.Fprintf(os.Stderr, "match-eval: --out is required\n")
		return 1
	}

	casesFile := findCasesFile(*corpusDir, *split)
	if casesFile == "" {
		fmt.Fprintf(os.Stderr, "match-eval: no cases file found for split %q in %s\n", *split, *corpusDir)
		return 1
	}

	cases, err := loadCases(casesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: load cases: %v\n", err)
		return 1
	}

	products, err := loadCatalogueSnapshot(*corpusDir, *split)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: load catalogue snapshot: %v\n", err)
		return 1
	}

	idx := productmatch.NewIndex(products)
	matchOpts := productmatch.DefaultMatchOptions()

	outcomes := make([]matcheval.Outcome, len(cases))
	for i, c := range cases {
		row := &productmatch.Row{
			Name:          c.Row.Cells["name"],
			Barcode:       c.Row.Cells["barcode"],
			SKU:           c.Row.Cells["code"],
			Concentration: c.Row.Cells["strength"],
			DosageForm:    c.Row.Cells["form"],
			Unit:          c.Row.Cells["unit"],
		}
		if row.SKU == "" {
			row.SKU = c.Row.Cells["sku"]
		}

		res := idx.Match(row, matchOpts)

		o := matcheval.Outcome{
			CaseID: c.ID,
			Path:   string(res.Level),
		}

		if res.Level.Settled() {
			o.State = "accepted"
			id := res.ProductID
			o.MatchedProductID = &id
			if p, ok := idx.Lookup(id); ok {
				candFacts := p.Facts()
				rowFacts := productmatch.ExtractRowFacts(row, nil)
				comp := productmatch.CompareFacts(rowFacts, candFacts)
				for _, h := range comp.Hard {
					o.HardConflictViolations = append(o.HardConflictViolations, string(h))
				}
			}
			if c.Label.Expect == "conflict" && len(o.HardConflictViolations) == 0 {
				o.HardConflictViolations = []string{"unexpected_acceptance_under_conflict"}
			}
		} else {
			switch res.Level {
			case productmatch.MatchAmbiguous:
				o.State = "ambiguous"
			case productmatch.MatchReview:
				o.State = "needs_review"
			default:
				if len(res.Candidates) > 0 {
					o.State = "needs_review"
				} else {
					o.State = "no_candidate"
				}
			}
		}

		topCount := len(res.Candidates)
		if topCount > 20 {
			topCount = 20
		}
		o.TopCandidates = make([]int64, topCount)
		for j := 0; j < topCount; j++ {
			o.TopCandidates[j] = res.Candidates[j].ProductID
		}

		outcomes[i] = o
	}

	reportOutcomes := outcomes
	var aiStats *matcheval.AIStats

	if *ai {
		finalOutcomes, stats, err := runAIEvaluation(
			cases,
			outcomes,
			idx,
			*configID,
			*promptVer,
			*aiSim,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: ai enhancement failed: %v\n", err)
			return 1
		}
		reportOutcomes = finalOutcomes
		aiStats = &stats
	}

	report := matcheval.Score(cases, reportOutcomes)
	report.Engine = *engine
	report.Corpus = *corpusDir
	report.Split = *split
	if *policy != "" {
		report.Policy = *policy
	}
	if aiStats != nil {
		report.Overall.AI = aiStats
	}

	if err := writeJSONReport(*outPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: write json report: %v\n", err)
		return 1
	}

	if *mdPath != "" {
		if err := writeMarkdownReport(*mdPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: write markdown report: %v\n", err)
			return 1
		}
	}

	// Exit codes: 0 pass, 2 fail, 3 insufficient, 1 error
	hasFail := false
	hasInsufficient := false
	for gate, res := range report.Gates {
		if *split == "adversarial" && gate != "SC-002" {
			continue
		}
		if res == matcheval.GateFail {
			hasFail = true
		} else if res == matcheval.GateInsufficientSample {
			hasInsufficient = true
		}
	}

	if hasFail {
		return 2
	}
	if hasInsufficient {
		return 3
	}
	return 0
}
