package main

// match-eval scores the matcher against labelled cases.
//
// Three things make the numbers honest, and each was missing before:
//
//   - Every case is matched against the catalogue it was written for. The
//     cross-script cases are Latin supplier lines whose answers carry that
//     exact Latin text in the catalogue's English column, so they run against
//     an index with the English column blanked; otherwise they measure string
//     equality. The egypt suite runs against an index with its withheld
//     products removed, which is what makes its negatives negative.
//   - The production path is measured: Index.Decide under a Policy, the same
//     call the matching service makes, rather than a legacy wrapper.
//   - --explain writes the wrong acceptances and the refused correct answers
//     with their reasons, because a precision figure without its failures
//     cannot be acted on.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func runMatchEval(args []string) int {
	fs := flag.NewFlagSet("match-eval", flag.ContinueOnError)

	corpusDir := fs.String("corpus", "test/corpus", "Path to corpus directory")
	split := fs.String("split", "tuning", "Corpus split (tuning|heldout|adversarial|egypt)")
	engine := fs.String("engine", "current", "Label recorded in the report; the binary being run is what is measured")
	policyPath := fs.String("policy", "", "Optional policy JSON file (defaults to productmatch.DefaultPolicy)")
	ai := fs.Bool("ai", false, "Run AI enhancement stage")
	configID := fs.String("config", "pm-default-v8", "AI configuration ID")
	promptVer := fs.String("prompt", "v8", "Prompt version (v7|v8)")
	aiSim := fs.String("ai-sim", "", "Simulator mode (fast|slow|down|budget|malformed|duplicate_refs|not_offered)")
	outPath := fs.String("out", "", "Output path for JSON report (required)")
	mdPath := fs.String("md", "", "Output path for Markdown report (optional)")
	explainPath := fs.String("explain", "", "Write wrong acceptances and refused correct answers to this file")
	explainN := fs.Int("explain-n", 60, "Examples per explain category")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: parse flags: %v\n", err)
		return 1
	}
	if *outPath == "" {
		fmt.Fprintf(os.Stderr, "match-eval: --out is required\n")
		return 1
	}

	pol := productmatch.DefaultPolicy()
	if *policyPath != "" {
		raw, err := os.ReadFile(*policyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: read policy: %v\n", err)
			return 1
		}
		if err := json.Unmarshal(raw, &pol); err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: parse policy: %v\n", err)
			return 1
		}
	}

	products, err := loadCatalogueSnapshot(*corpusDir, *split)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: load catalogue snapshot: %v\n", err)
		return 1
	}

	var cases []matcheval.Case
	if *split == "egypt" {
		cases = matcheval.EgyptianCases(products)
	} else {
		casesFile := findCasesFile(*corpusDir, *split)
		if casesFile == "" {
			fmt.Fprintf(os.Stderr, "match-eval: no cases file found for split %q in %s\n", *split, *corpusDir)
			return 1
		}
		if cases, err = loadCases(casesFile); err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: load cases: %v\n", err)
			return 1
		}
	}

	indexes := newEvalIndexes(products)
	outcomes, decisions := decideCases(cases, indexes, pol)

	reportOutcomes := outcomes
	var aiStats *matcheval.AIStats
	if *ai {
		finalOutcomes, stats, err := runAIEvaluation(cases, outcomes, indexes.get(""), *configID, *promptVer, *aiSim)
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
	report.Policy = pol.Version
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
	if *explainPath != "" {
		if err := writeExplain(*explainPath, cases, decisions, indexes, *explainN); err != nil {
			fmt.Fprintf(os.Stderr, "match-eval: write explain: %v\n", err)
			return 1
		}
	}
	printSummary(report)

	hasFail, hasInsufficient := false, false
	for gate, res := range report.Gates {
		if *split == "adversarial" && gate != "SC-002" {
			continue
		}
		switch res {
		case matcheval.GateFail:
			hasFail = true
		case matcheval.GateInsufficientSample:
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

// evalIndexes builds each catalogue variant once, on first use.
type evalIndexes struct {
	products []productmatch.MasterProduct
	mu       sync.Mutex
	built    map[string]*productmatch.Index
}

func newEvalIndexes(products []productmatch.MasterProduct) *evalIndexes {
	return &evalIndexes{products: products, built: map[string]*productmatch.Index{}}
}

// snapshotFor names the catalogue variant a case must be matched against.
func snapshotFor(c matcheval.Case) string {
	switch {
	case c.CatalogSnapshot == matcheval.EgyptSnapshot:
		return matcheval.EgyptSnapshot
	case c.TemplateFamily == "admin-cross":
		return "blank-en"
	}
	return ""
}

func (e *evalIndexes) get(name string) *productmatch.Index {
	e.mu.Lock()
	defer e.mu.Unlock()
	if idx, ok := e.built[name]; ok {
		return idx
	}
	src := make([]productmatch.MasterProduct, 0, len(e.products))
	for _, p := range e.products {
		switch name {
		case "blank-en":
			p.NameEN = ""
		case matcheval.EgyptSnapshot:
			if matcheval.EgyptWithheld(p.ID) {
				continue
			}
		}
		src = append(src, p)
	}
	idx := productmatch.NewIndex(src)
	e.built[name] = idx
	return idx
}

func caseRow(c matcheval.Case) *productmatch.Row {
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
	return row
}

// decideCases runs the production decision for every case, in parallel.
func decideCases(cases []matcheval.Case, ix *evalIndexes, pol productmatch.Policy) ([]matcheval.Outcome, []matchflow.RowDecision) {
	outcomes := make([]matcheval.Outcome, len(cases))
	decisions := make([]matchflow.RowDecision, len(cases))

	// Build every index up front so the workers never wait on one another.
	for _, c := range cases {
		ix.get(snapshotFor(c))
	}

	var wg sync.WaitGroup
	next := make(chan int, 256)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				c := cases[i]
				idx := ix.get(snapshotFor(c))
				d := idx.Decide(caseRow(c), pol)
				decisions[i] = d
				outcomes[i] = outcomeOf(c, d)
			}
		}()
	}
	for i := range cases {
		next <- i
	}
	close(next)
	wg.Wait()
	return outcomes, decisions
}

func outcomeOf(c matcheval.Case, d matchflow.RowDecision) matcheval.Outcome {
	o := matcheval.Outcome{
		CaseID:              c.ID,
		State:               string(d.State),
		Basis:               string(d.Basis),
		Path:                string(d.Basis),
		AchievedGranularity: string(d.Granularity.Achieved),
	}
	for _, r := range d.Reasons {
		o.Reasons = append(o.Reasons, r.Code)
	}
	if (d.State == matchflow.StateAccepted || d.State == matchflow.StateConfirmed) && d.ProductID != nil {
		id := *d.ProductID
		o.MatchedProductID = &id
		// An independent check, not the gate's own: a case LABELLED as a
		// conflict that was accepted anyway.
		if c.Label.Expect == "conflict" {
			o.HardConflictViolations = []string{"accepted_labelled_conflict"}
		}
	}
	for j, cand := range d.Candidates {
		if j >= 20 {
			break
		}
		o.TopCandidates = append(o.TopCandidates, cand.ProductID)
	}
	return o
}

func printSummary(r matcheval.Report) {
	o := r.Overall
	fmt.Printf("cases=%d resolvable=%d accepted=%d correct=%d precision=%.4f (lb95 %.4f) coverage=%.4f recall@cands=%.4f absent/ambiguous accepted=%d\n",
		o.NCases, o.NResolvable, o.Accepted, o.AcceptedCorrect, o.Precision, o.PrecisionLB95,
		o.CoverageResolvable, o.RecallAt20, o.AbsentOrAmbiguousAccepted)
	for g, res := range r.Gates {
		fmt.Printf("  %s: %s\n", g, res)
	}
}
