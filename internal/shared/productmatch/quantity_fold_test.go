package productmatch

import "testing"

// Two multiplications in one name used to panic: the left factor's offset was
// taken from the input and applied to the rebuilt output, which is shorter once
// the first fold has happened. Matching runs on worker goroutines, so the panic
// ended the server process in the middle of an admin import.
func TestFoldPackMultipliers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"اليرجيل 4مجم 20*10 قرص", "اليرجيل 4مجم 200 قرص"},
		{"10*10 علبة 5*6", "100 علبة 30"},
		{"2*3*4 tabs", "24 tabs"},
		// The shape that crashed in production: one multiplication, read twice
		// because the matcher joins the Arabic and English names.
		{"شاور جل 3×1 250 مل شاور جل 3×1 250 مل", "شاور جل 3 250 مل شاور جل 3 250 مل"},
		{"20 x 10 caps", "200 caps"},
		{"b12 x 30", "b12 x 30"},
		{"vitax 10", "vitax 10"},
		{"سوبر كيدز 2*1 شامبو 500 مل", "سوبر كيدز 2 شامبو 500 مل"},
		// Factors past the bound are not a pack, and must not overflow into one.
		{"99999999999*99999999999", "99999999999*99999999999"},
		{"*5", "*5"},
		{"5*", "5*"},
	}
	for _, c := range cases {
		if got := foldPackMultipliers(c.in); got != c.want {
			t.Errorf("foldPackMultipliers(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func FuzzFoldPackMultipliers(f *testing.F) {
	for _, seed := range []string{"10*10 علبة 5*6", "2*3*4", "1x2x3 x", "٣*٤ 10×10", "x*X×"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = foldPackMultipliers(s)
		_ = readQuantities(s)
	})
}

// A panic while scoring one row costs that row, never the batch.
func TestMatchAllContainsRowPanics(t *testing.T) {
	idx := NewIndex([]MasterProduct{{ID: 1, NameAR: "بانادول 500 مجم 24 قرص"}})
	rows := []*Row{{Name: "بانادول 500 مجم"}, {Name: "10*10 علبة 5*6 بانادول"}}
	for _, workers := range []int{1, 4} {
		res := MatchAll(idx, rows, DefaultMatchOptions(), workers)
		if len(res) != len(rows) {
			t.Fatalf("workers=%d: got %d results", workers, len(res))
		}
	}
}
