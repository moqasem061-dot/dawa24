package catalog

import "testing"

// Over the ceiling, the unresolved (needs-review / unmatched) rows are the
// ones kept: an unchecked settled row keeps a match, an unasked unresolved
// row keeps nothing.
func TestCeilingKeepsUnresolvedRowsFirst(t *testing.T) {
	ceiling := adjudicationCeiling()
	settled := make([]askable, ceiling)
	for i := range settled {
		settled[i] = askable{at: i, settled: true, guess: int64(i + 1)}
	}
	unresolved := make([]askable, 10)
	for i := range unresolved {
		unresolved[i] = askable{at: ceiling + i}
	}

	var stats MatchStats
	got := capToAdjudicationCeiling(settled, unresolved, &stats)
	if len(got) != ceiling {
		t.Fatalf("kept %d rows, want the ceiling %d", len(got), ceiling)
	}
	for i := range unresolved {
		if got[i].settled {
			t.Fatalf("row %d is settled; unresolved rows must be kept first", i)
		}
	}
	if !stats.CeilingHit || stats.Unmatched != len(unresolved) {
		t.Errorf("stats = %+v, want ceiling hit and %d unmatched", stats, len(unresolved))
	}

	// Under the ceiling, everything is asked.
	stats = MatchStats{}
	if got := capToAdjudicationCeiling(settled[:5], unresolved, &stats); len(got) != 15 {
		t.Errorf("under the ceiling kept %d rows, want all 15", len(got))
	}
}
