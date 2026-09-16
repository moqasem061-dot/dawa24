package sheet

import (
	"os"
	"strings"
	"testing"
)

// TestBIFFSharedStringsAcrossContinue is the file both third-party decoders
// lost most of.
//
// الدقهليه-1.XLS is 764 rows of a Cairo distributor's price list in four
// columns. Its shared-string table spans four CONTINUE records, and a string
// split at one of those boundaries carries a fresh encoding byte for its
// remainder. shakinm/xlsReader panics on it; extrame/xls returns the first 155
// strings and empty cells for the other 609 — and the vendor's import screen
// then showed 654 of 764 rows unmatched, because 609 of them had no name to
// match with.
func TestBIFFSharedStringsAcrossContinue(t *testing.T) {
	data, err := os.ReadFile("testdata/sst-continue.xls")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	rows, err := ReadRows(data, "sst-continue.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(rows) != 764 {
		t.Fatalf("rows = %d, want 764", len(rows))
	}

	// Every row of every column is populated. The failure this pins is not a
	// wrong value, it is a missing one, so the count is the assertion.
	named := 0
	for _, row := range rows {
		if len(row) > 2 && CleanCell(row[2]) != "" {
			named++
		}
	}
	if named != 764 {
		t.Errorf("named rows = %d, want 764 (the CONTINUE boundary lost the rest)", named)
	}

	// Spot values on both sides of the boundary the old decoders stopped at,
	// checked against what Excel shows.
	for _, want := range []struct {
		row  int
		cols []string
	}{
		{0, []string{"نسبة الخصم", "سعر الجمهور", "اسم المنتج", "كود المنتج"}},
		{1, []string{"26", "180", "ابيتويد قرص-2", "6775"}},
		{154, []string{"33", "135", "اوكتاترون3شريـــــــــط س ج 0", "11690"}},
		{155, []string{"31", "70", "اوكسيتوفليكس 10 وحدة/مل 5 امبولات س ج", "12403"}},
		{163, []string{"30", "411", "اوميجابريس 2 مجم قرص", "9813"}},
		{763, nil},
	} {
		if want.cols == nil {
			continue
		}
		got := rows[want.row]
		for c, w := range want.cols {
			g := ""
			if c < len(got) {
				g = CleanCell(got[c])
			}
			if g != w {
				t.Errorf("row %d col %d: got %q want %q", want.row, c, g, w)
			}
		}
	}

	// Raw record bytes leaking into a cell is the other way this file used to
	// fail, and it is invisible without looking for it.
	for i, row := range rows {
		for c, cell := range row {
			if strings.ContainsRune(cell, 0) {
				t.Fatalf("row %d col %d holds undecoded bytes: %q", i, c, cell)
			}
		}
	}
}

// TestCompoundFileRejectsRubbish checks the container reader refuses what is
// not one, rather than reading whatever the bytes happen to say.
func TestCompoundFileRejectsRubbish(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		[]byte("not a compound file at all"),
		make([]byte, 600),
	} {
		if _, err := openCompoundFile(in); err == nil {
			t.Errorf("openCompoundFile(%d bytes) = nil error, want refusal", len(in))
		}
	}
}

// TestDecodeRK pins Excel's packed number, which encodes four different things
// in two bits.
func TestDecodeRK(t *testing.T) {
	cases := []struct {
		rk   uint32
		want float64
	}{
		{0x00000002, 0},    // integer 0
		{0x0000000A, 2},    // integer 2
		{0x0000000B, 0.02}, // integer 2, divided by 100
		{0x3FF00000, 1},    // float 1.0
		{0x3FF00001, 0.01}, // float 1.0, divided by 100
		{0xFFFFFFFE, -1},   // negative integer
	}
	for _, tc := range cases {
		if got := decodeRK(tc.rk); got != tc.want {
			t.Errorf("decodeRK(%#08x) = %v, want %v", tc.rk, got, tc.want)
		}
	}
}
