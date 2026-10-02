package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

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

func loadCatalogueSnapshot(dir, split string) ([]productmatch.MasterProduct, error) {
	candidates := []string{
		filepath.Join(dir, split, "catalogue.jsonl"),
		filepath.Join(dir, split, "catalogue.jsonl.gz"),
		filepath.Join(dir, fmt.Sprintf("catalogue_%s.jsonl", split)),
		filepath.Join(dir, fmt.Sprintf("catalogue_%s.jsonl.gz", split)),
		filepath.Join(dir, "catalogue.jsonl"),
		filepath.Join(dir, "catalogue.jsonl.gz"),
		filepath.Join("test", "corpus", "catalogue.jsonl.gz"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return readCatalogueFile(p)
		}
	}
	return nil, fmt.Errorf("no catalogue snapshot found in %s for split %s", dir, split)
}

func readCatalogueFile(path string) ([]productmatch.MasterProduct, error) {
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

	var products []productmatch.MasterProduct
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var p struct {
			ID            int64  `json:"id"`
			NameAR        string `json:"name_ar"`
			NameEN        string `json:"name_en"`
			Name          string `json:"name"`
			SKU           string `json:"sku"`
			Barcode       string `json:"barcode"`
			Scientific    string `json:"scientific"`
			DosageForm    string `json:"dosage_form"`
			Concentration string `json:"concentration"`
			Unit          string `json:"unit"`
			Manufacturer  string `json:"manufacturer"`
			PublicPrice   string `json:"price"`

			I int64  `json:"i"`
			A string `json:"a"`
			E string `json:"e"`
			K string `json:"k"`
			B string `json:"b"`
			S string `json:"s"`
			F string `json:"f"`
			C string `json:"c"`
			U string `json:"u"`
			M string `json:"m"`
			P string `json:"p"`
		}
		if err := json.Unmarshal(line, &p); err != nil {
			return nil, err
		}
		if p.ID == 0 && p.I != 0 {
			p.ID = p.I
			p.NameAR = p.A
			p.NameEN = p.E
			p.SKU = p.K
			p.Barcode = p.B
			p.Scientific = p.S
			p.DosageForm = p.F
			p.Concentration = p.C
			p.Unit = p.U
			p.Manufacturer = p.M
			p.PublicPrice = p.P
		}
		if p.NameAR == "" && p.Name != "" {
			p.NameAR = p.Name
		}
		products = append(products, productmatch.MasterProduct{
			ID:            p.ID,
			NameAR:        p.NameAR,
			NameEN:        p.NameEN,
			SKU:           p.SKU,
			Barcode:       p.Barcode,
			Scientific:    p.Scientific,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Unit:          p.Unit,
			Manufacturer:  p.Manufacturer,
			PublicPrice:   p.PublicPrice,
		})
	}
	return products, sc.Err()
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
	fmt.Fprintf(&sb, "# Evaluation Report: %s (%s split)\n\n", r.Engine, r.Split)
	fmt.Fprintf(&sb, "- **Corpus**: `%s`\n", r.Corpus)
	fmt.Fprintf(&sb, "- **Total Cases**: %d\n", r.Overall.NCases)
	fmt.Fprintf(&sb, "- **Resolvable Cases**: %d\n", r.Overall.NResolvable)
	fmt.Fprintf(&sb, "- **Accepted**: %d (Correct: %d)\n", r.Overall.Accepted, r.Overall.AcceptedCorrect)
	fmt.Fprintf(&sb, "- **Precision**: %.2f%% (95%% LB: %.2f%%)\n", r.Overall.Precision*100, r.Overall.PrecisionLB95*100)
	fmt.Fprintf(&sb, "- **Coverage (Resolvable)**: %.2f%%\n", r.Overall.CoverageResolvable*100)
	fmt.Fprintf(&sb, "- **Candidate Recall@20**: %.2f%%\n", r.Overall.RecallAt20*100)
	fmt.Fprintf(&sb, "- **Hard Conflict Violations**: %d\n\n", len(r.Overall.HardConflictViolations))

	if r.Overall.AI != nil {
		sb.WriteString("## AI Enhancement Metrics\n\n")
		fmt.Fprintf(&sb, "- **Gain (New Correct)**: %d\n", r.Overall.AI.GainNewCorrect)
		fmt.Fprintf(&sb, "- **New Wrong**: %d\n", r.Overall.AI.NewWrong)
		fmt.Fprintf(&sb, "- **Lost Correct**: %d\n", r.Overall.AI.LostCorrect)
		fmt.Fprintf(&sb, "- **Net Gain Rate**: %.2f%%\n", r.Overall.AI.NetGainRate*100)
		fmt.Fprintf(&sb, "- **AI-Added Precision (95%% LB)**: %.2f%%\n", r.Overall.AI.AIAddedPrecisionLB95*100)
		fmt.Fprintf(&sb, "- **Attempts**: %d\n", r.Overall.AI.Attempts)
		fmt.Fprintf(&sb, "- **Tokens**: %d\n", r.Overall.AI.Tokens)
		fmt.Fprintf(&sb, "- **Cost**: $%.4f (Known: %t)\n", r.Overall.AI.Cost, r.Overall.AI.CostKnown)
		fmt.Fprintf(&sb, "- **Cost per Verified Gain**: $%.4f\n\n", r.Overall.AI.CostPerVerifiedGain)
	}

	sb.WriteString("## Quality Gates\n\n")
	sb.WriteString("| Gate | Description | Status |\n")
	sb.WriteString("|---|---|---|\n")
	for gate, res := range r.Gates {
		var statusText string
		switch res {
		case matcheval.GatePass:
			statusText = "✓ PASS"
		case matcheval.GateFail:
			statusText = "✗ FAIL"
		default:
			statusText = "⚠ INSUFFICIENT SAMPLE"
		}
		fmt.Fprintf(&sb, "| %s | Gate requirement | %s |\n", gate, statusText)
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
