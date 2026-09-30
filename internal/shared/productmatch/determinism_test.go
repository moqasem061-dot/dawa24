package productmatch

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"
)

func TestDeterminism_MatchAllAndDecideAll(t *testing.T) {
	cat := syntheticCatalogue(2000)
	idx := NewIndex(cat)

	rows := syntheticRows(500, cat)
	rng := rand.New(rand.NewSource(42))
	rng.Shuffle(len(rows), func(i, j int) {
		rows[i], rows[j] = rows[j], rows[i]
	})

	pol := DefaultPolicy()
	opts := DefaultMatchOptions()

	// 1. Run DecideAll with workers 1, 4, 16
	dec1 := DecideAll(idx, rows, pol, 1)
	dec4 := DecideAll(idx, rows, pol, 4)
	dec16 := DecideAll(idx, rows, pol, 16)

	if len(dec1) != len(rows) || len(dec4) != len(rows) || len(dec16) != len(rows) {
		t.Fatalf("length mismatch: rows=%d, w1=%d, w4=%d, w16=%d",
			len(rows), len(dec1), len(dec4), len(dec16))
	}

	for i := range rows {
		json1, err1 := json.Marshal(dec1[i])
		if err1 != nil {
			t.Fatalf("row %d: json.Marshal w1 failed: %v", i, err1)
		}
		json4, err4 := json.Marshal(dec4[i])
		if err4 != nil {
			t.Fatalf("row %d: json.Marshal w4 failed: %v", i, err4)
		}
		json16, err16 := json.Marshal(dec16[i])
		if err16 != nil {
			t.Fatalf("row %d: json.Marshal w16 failed: %v", i, err16)
		}

		if !bytes.Equal(json1, json4) {
			t.Fatalf("row %d: worker 1 and worker 4 RowDecision JSON differ:\nw1: %s\nw4: %s",
				i, string(json1), string(json4))
		}
		if !bytes.Equal(json1, json16) {
			t.Fatalf("row %d: worker 1 and worker 16 RowDecision JSON differ:\nw1: %s\nw16: %s",
				i, string(json1), string(json16))
		}
	}

	// 2. Also verify MatchAll with workers 1, 4, 16
	match1 := MatchAll(idx, rows, opts, 1)
	match4 := MatchAll(idx, rows, opts, 4)
	match16 := MatchAll(idx, rows, opts, 16)

	for i := range rows {
		if match1[i].ProductID != match4[i].ProductID || match1[i].Level != match4[i].Level {
			t.Fatalf("row %d MatchAll w1 vs w4 mismatch: w1=%+v, w4=%+v", i, match1[i], match4[i])
		}
		if match1[i].ProductID != match16[i].ProductID || match1[i].Level != match16[i].Level {
			t.Fatalf("row %d MatchAll w1 vs w16 mismatch: w1=%+v, w16=%+v", i, match1[i], match16[i])
		}
	}
}
