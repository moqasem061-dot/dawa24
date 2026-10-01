package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
)

func runMatchCorpusSplit(args []string) int {
	fs := flag.NewFlagSet("match-corpus-split", flag.ContinueOnError)
	inPath := fs.String("in", "", "Input cases JSONL file or directory containing cases.jsonl (required)")
	outDir := fs.String("out", "", "Output directory for split JSONL files (required)")
	seed := fs.Int64("seed", 42, "PRNG seed for reproducible family splitting")
	ratio := fs.Float64("ratio", 0.60, "Target proportion of cases in tuning split (default 0.60)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: parse flags: %v\n", err)
		return 1
	}

	if *inPath == "" || *outDir == "" {
		fmt.Fprintf(os.Stderr, "match-corpus-split: both --in and --out are required\n")
		return 1
	}

	// Resolve input file
	inputFile := *inPath
	fi, err := os.Stat(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: stat --in: %v\n", err)
		return 1
	}
	if fi.IsDir() {
		inputFile = filepath.Join(*inPath, "cases.jsonl")
	}

	cases, err := loadCases(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: load input cases: %v\n", err)
		return 1
	}
	if len(cases) == 0 {
		fmt.Fprintf(os.Stderr, "match-corpus-split: no cases found in %s\n", inputFile)
		return 1
	}

	tuning, heldout, err := SplitCorpus(cases, *seed, *ratio)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: split error: %v\n", err)
		return 1
	}

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: create output dir: %v\n", err)
		return 1
	}

	tuningPath := filepath.Join(*outDir, "cases_tuning.jsonl")
	heldoutPath := filepath.Join(*outDir, "cases_heldout.jsonl")

	if err := writeCasesFile(tuningPath, tuning); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: write tuning cases: %v\n", err)
		return 1
	}
	if err := writeCasesFile(heldoutPath, heldout); err != nil {
		fmt.Fprintf(os.Stderr, "match-corpus-split: write heldout cases: %v\n", err)
		return 1
	}

	fmt.Printf("Split complete: %d total cases -> %d tuning (%.1f%%), %d heldout (%.1f%%) in %s\n",
		len(cases),
		len(tuning), float64(len(tuning))/float64(len(cases))*100,
		len(heldout), float64(len(heldout))/float64(len(cases))*100,
		*outDir,
	)
	return 0
}

// SplitCorpus partitions cases into tuning and heldout splits such that
// NO template family and NO expression family crosses splits (research R17).
func SplitCorpus(cases []matcheval.Case, seed int64, ratio float64) ([]matcheval.Case, []matcheval.Case, error) {
	if len(cases) == 0 {
		return nil, nil, nil
	}

	dsu := newDisjointSet()

	// 1. Group connected components of template and expression families
	for i, c := range cases {
		tf := "tf:" + c.TemplateFamily
		ef := "ef:" + c.ExpressionFamily
		dsu.union(tf, ef)
		caseNode := fmt.Sprintf("case:%d", i)
		dsu.union(caseNode, tf)
	}

	// 2. Aggregate cases by root component
	components := make(map[string][]int)
	for i := range cases {
		caseNode := fmt.Sprintf("case:%d", i)
		root := dsu.find(caseNode)
		components[root] = append(components[root], i)
	}

	type componentGroup struct {
		indices []int
	}

	var groups []componentGroup
	for _, idxs := range components {
		groups = append(groups, componentGroup{indices: idxs})
	}

	// Sort groups deterministically before shuffling
	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].indices) != len(groups[j].indices) {
			return len(groups[i].indices) > len(groups[j].indices)
		}
		return groups[i].indices[0] < groups[j].indices[0]
	})

	// Shuffle with seed
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(groups), func(i, j int) {
		groups[i], groups[j] = groups[j], groups[i]
	})

	targetTuning := int(float64(len(cases)) * ratio)
	currentTuning := 0

	var tuningIndices, heldoutIndices []int

	for _, g := range groups {
		if currentTuning < targetTuning || len(heldoutIndices) == 0 {
			tuningIndices = append(tuningIndices, g.indices...)
			currentTuning += len(g.indices)
		} else {
			heldoutIndices = append(heldoutIndices, g.indices...)
		}
	}

	// Fallback if all cases landed in tuning
	if len(heldoutIndices) == 0 && len(groups) > 1 {
		lastGroup := groups[len(groups)-1]
		tuningIndices = tuningIndices[:len(tuningIndices)-len(lastGroup.indices)]
		heldoutIndices = lastGroup.indices
	}

	tuningCases := make([]matcheval.Case, len(tuningIndices))
	for i, idx := range tuningIndices {
		c := cases[idx]
		c.Split = "tuning"
		tuningCases[i] = c
	}

	heldoutCases := make([]matcheval.Case, len(heldoutIndices))
	for i, idx := range heldoutIndices {
		c := cases[idx]
		c.Split = "heldout"
		heldoutCases[i] = c
	}

	// Invariant verification: assert disjoint sets
	tuningTF := make(map[string]struct{})
	tuningEF := make(map[string]struct{})
	for _, c := range tuningCases {
		if c.TemplateFamily != "" {
			tuningTF[c.TemplateFamily] = struct{}{}
		}
		if c.ExpressionFamily != "" {
			tuningEF[c.ExpressionFamily] = struct{}{}
		}
	}

	for _, c := range heldoutCases {
		if _, ok := tuningTF[c.TemplateFamily]; ok && c.TemplateFamily != "" {
			return nil, nil, fmt.Errorf("invariant violation: template family %q crosses splits", c.TemplateFamily)
		}
		if _, ok := tuningEF[c.ExpressionFamily]; ok && c.ExpressionFamily != "" {
			return nil, nil, fmt.Errorf("invariant violation: expression family %q crosses splits", c.ExpressionFamily)
		}
	}

	return tuningCases, heldoutCases, nil
}

func writeCasesFile(path string, cases []matcheval.Case) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, c := range cases {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}

type disjointSet struct {
	parent map[string]string
}

func newDisjointSet() *disjointSet {
	return &disjointSet{parent: make(map[string]string)}
}

func (d *disjointSet) find(x string) string {
	p, ok := d.parent[x]
	if !ok || p == x {
		d.parent[x] = x
		return x
	}
	d.parent[x] = d.find(p)
	return d.parent[x]
}

func (d *disjointSet) union(x, y string) {
	rx := d.find(x)
	ry := d.find(y)
	if rx != ry {
		d.parent[rx] = ry
	}
}
