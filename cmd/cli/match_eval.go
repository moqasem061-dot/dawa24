package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/test/corpus"
)

func runMatchEval(args []string) int {
	fs := flag.NewFlagSet("match-eval", flag.ContinueOnError)

	corpusDir := fs.String("corpus", "test/corpus", "Path to corpus directory")
	split := fs.String("split", "tuning", "Corpus split (tuning|heldout|adversarial)")
	engine := fs.String("engine", "current", "Engine to evaluate (current|baseline)")
	policy := fs.String("policy", "", "Optional policy file or version")
	ai := fs.Bool("ai", false, "Run AI enhancement stage")
	configID := fs.String("config", "", "AI configuration ID")
	budget := fs.String("budget", "", "AI budget file")
	outPath := fs.String("out", "", "Output path for JSON report (required)")
	mdPath := fs.String("md", "", "Output path for Markdown report (optional)")

	_ = configID
	_ = budget
	_ = policy

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "match-eval: parse flags: %v\n", err)
		return 1
	}

	if *ai {
		fmt.Fprintf(os.Stderr, "match-eval: --ai is not implemented yet\n")
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

	products, err := corpus.LoadSnapshotDir(*corpusDir)
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

		if res.Matched() {
			o.State = "accepted"
			id := res.ProductID
			o.MatchedProductID = &id
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

	report := matcheval.Score(cases, outcomes)
	report.Engine = *engine
	report.Corpus = *corpusDir
	report.Split = *split
	if *policy != "" {
		report.Policy = *policy
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
	for _, res := range report.Gates {
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

func findCasesFile(dir, split string) string {
	candidates := []string{
		filepath.Join(dir, fmt.Sprintf("cases_%s.jsonl.gz", split)),
		filepath.Join(dir, fmt.Sprintf("cases_%s.jsonl", split)),
		filepath.Join(dir, split, "cases.jsonl"),
		filepath.Join(dir, split, "cases.jsonl.gz"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func loadCases(path string) ([]matcheval.Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		r = zr
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var cases []matcheval.Case
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var c matcheval.Case
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

func writeJSONReport(path string, report matcheval.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func writeMarkdownReport(path string, r matcheval.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Evaluation Report: %s (%s split)\n\n", r.Engine, r.Split))
	sb.WriteString(fmt.Sprintf("- **Corpus**: `%s`\n", r.Corpus))
	sb.WriteString(fmt.Sprintf("- **Total Cases**: %d\n", r.Overall.NCases))
	sb.WriteString(fmt.Sprintf("- **Resolvable Cases**: %d\n", r.Overall.NResolvable))
	sb.WriteString(fmt.Sprintf("- **Accepted**: %d (Correct: %d)\n", r.Overall.Accepted, r.Overall.AcceptedCorrect))
	sb.WriteString(fmt.Sprintf("- **Precision**: %.2f%% (95%% LB: %.2f%%)\n", r.Overall.Precision*100, r.Overall.PrecisionLB95*100))
	sb.WriteString(fmt.Sprintf("- **Coverage (Resolvable)**: %.2f%%\n", r.Overall.CoverageResolvable*100))
	sb.WriteString(fmt.Sprintf("- **Candidate Recall@20**: %.2f%%\n", r.Overall.RecallAt20*100))
	sb.WriteString(fmt.Sprintf("- **Hard Conflict Violations**: %d\n\n", len(r.Overall.HardConflictViolations)))

	sb.WriteString("## Quality Gates\n\n")
	sb.WriteString("| Gate | Description | Status |\n")
	sb.WriteString("|---|---|---|\n")
	for gate, res := range r.Gates {
		statusText := string(res)
		if res == matcheval.GatePass {
			statusText = "✓ PASS"
		} else if res == matcheval.GateFail {
			statusText = "✗ FAIL"
		} else {
			statusText = "⚠ INSUFFICIENT SAMPLE"
		}
		sb.WriteString(fmt.Sprintf("| %s | Gate requirement | %s |\n", gate, statusText))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
