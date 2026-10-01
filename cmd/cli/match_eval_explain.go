package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/matcheval"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// writeExplain lists the two kinds of failure with enough context to act on:
// wrong acceptances (precision) and correct answers that were not accepted
// (coverage), each grouped by the reason the engine gave.
func writeExplain(path string, cases []matcheval.Case, decs []matchflow.RowDecision, ix *evalIndexes, perCat int) error {
	type example struct{ line string }
	wrong := map[string][]string{}
	refused := map[string][]string{}
	wrongN, refusedN := map[string]int{}, map[string]int{}

	name := func(idx *productmatch.Index, id int64) string {
		if p, ok := idx.Lookup(id); ok {
			if p.NameEN != "" {
				return fmt.Sprintf("%d %q / %q", id, p.NameAR, p.NameEN)
			}
			return fmt.Sprintf("%d %q", id, p.NameAR)
		}
		return fmt.Sprintf("%d <not in index>", id)
	}
	reasonOf := func(d matchflow.RowDecision) string {
		parts := []string{string(d.State)}
		for _, r := range d.Reasons {
			parts = append(parts, r.Code)
		}
		return strings.Join(parts, ":")
	}
	isTarget := func(c matcheval.Case, id int64) bool {
		for _, t := range c.Label.Targets {
			if t == id {
				return true
			}
		}
		return false
	}

	for i, c := range cases {
		d := decs[i]
		idx := ix.get(snapshotFor(c))
		accepted := (d.State == matchflow.StateAccepted || d.State == matchflow.StateConfirmed) && d.ProductID != nil
		key := reasonOf(d) + " basis=" + string(d.Basis)
		switch {
		case accepted && (!c.Label.Resolvable || !isTarget(c, *d.ProductID)):
			wrongN[key]++
			if len(wrong[key]) < perCat {
				want := "(abstain)"
				if len(c.Label.Targets) > 0 {
					want = name(idx, c.Label.Targets[0])
				}
				wrong[key] = append(wrong[key], fmt.Sprintf("  [%s] row=%q\n      got=%s\n      want=%s",
					c.ID, c.Row.Cells["name"], name(idx, *d.ProductID), want))
			}
		case !accepted && c.Label.Resolvable:
			rank := "not-in-shortlist"
			for j, cand := range d.Candidates {
				if isTarget(c, cand.ProductID) {
					rank = fmt.Sprintf("rank=%d eligible=%v reasons=%v", j+1, cand.Eligible, cand.Reasons)
					break
				}
			}
			refusedN[key]++
			if len(refused[key]) < perCat {
				refused[key] = append(refused[key], fmt.Sprintf("  [%s] row=%q\n      want=%s (%s)",
					c.ID, c.Row.Cells["name"], name(idx, c.Label.Targets[0]), rank))
			}
		}
	}

	var b strings.Builder
	write := func(title string, n map[string]int, ex map[string][]string) {
		keys := make([]string, 0, len(n))
		total := 0
		for k, v := range n {
			keys = append(keys, k)
			total += v
		}
		sort.Slice(keys, func(i, j int) bool { return n[keys[i]] > n[keys[j]] })
		fmt.Fprintf(&b, "==================== %s: %d ====================\n", title, total)
		for _, k := range keys {
			fmt.Fprintf(&b, "\n--- %d × %s\n", n[k], k)
			for _, e := range ex[k] {
				b.WriteString(e + "\n")
			}
		}
		b.WriteString("\n")
	}
	write("WRONG ACCEPTANCES", wrongN, wrong)
	write("CORRECT ANSWER NOT ACCEPTED", refusedN, refused)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
