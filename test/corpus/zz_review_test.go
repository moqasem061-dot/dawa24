package corpus

import (
	"fmt"
	"sort"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestZZCrossReviewReasons(t *testing.T) {
	if !Available() {
		t.Skip()
	}
	products, _ := LoadSnapshot()
	labels, _ := CrossScriptLabels(products)
	idx := productmatch.NewIndex(BlankEnglish(products))
	pol := productmatch.DefaultPolicy()
	tally := map[string]int{}
	ex := map[string][]string{}
	for _, l := range labels {
		d := idx.Decide(l.Row, pol)
		if d.State == "accepted" || d.ProductID == nil || *d.ProductID != l.WantID {
			continue
		}
		k := string(d.State)
		for _, r := range d.Reasons {
			k += ":" + r.Code
		}
		tally[k]++
		if len(ex[k]) < 6 {
			p, _ := idx.Lookup(l.WantID)
			ex[k] = append(ex[k], fmt.Sprintf("%q [form=%q conc=%q] -> %q", l.Row.Name, l.Row.DosageForm, l.Row.Concentration, p.NameAR))
		}
	}
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return tally[keys[i]] > tally[keys[j]] })
	for _, k := range keys[:min(8, len(keys))] {
		fmt.Println(tally[k], k)
		for _, e := range ex[k] {
			fmt.Println("    ", e)
		}
	}
}
