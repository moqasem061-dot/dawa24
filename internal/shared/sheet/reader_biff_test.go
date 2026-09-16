package sheet

import (
	"os"
	"strings"
	"testing"
)

// A real distributor BIFF file the legacy decoder glued into unreadable blobs
// from row 273 on (Continue-record mishandling) and then refused outright.
// Excel opens it perfectly: 1012 rows of price list. The modern decoder must
// read every row, byte-identical to the reference values below.
func TestBIFFContinueHeavyFile(t *testing.T) {
	data, err := os.ReadFile(`../../../test/corpus/files/vendor-37.xls`)
	if err != nil {
		t.Skipf("corpus file missing: %v", err)
	}
	if Detect(data) != FormatXLS {
		t.Fatalf("detect = %s, want xls", Detect(data))
	}
	rows, err := ReadRows(data, "vendor-37.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(rows) < 1012 {
		t.Fatalf("rows = %d, want >= 1012", len(rows))
	}
	for _, want := range []struct {
		row  int
		cols []string
	}{
		{0, []string{"سعر الجمهور", "الخصم", "اسم الصنف", "كود الصنف"}},
		{140, []string{"24", "33", "اوتريفين 15مل. نقط كبار", "17034"}},
		{273, []string{"216", "29", "بريكسبيبرازول كلافيتا فارما 1 مجم 30 قرص", "12522"}},
		{274, []string{"216", "28", "بريكسبيبرازول كلافيتا فارما 2 مجم 30 قرص", "12523"}},
		{1010, []string{"53", "37", "يونيفنجاى 150 مجم 2 كبسولة", "331"}},
	} {
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
	// No undecodable residue anywhere: a NUL byte in a decoded cell means raw
	// record bytes leaked into the grid.
	for i, row := range rows {
		for j, cell := range row {
			if strings.ContainsRune(cell, 0) {
				t.Errorf("NUL residue at row %d col %d", i, j)
			}
		}
	}
}

// A BIFF file the legacy decoder already handled must decode identically
// through the modern path: same headers, same row count, same values.
func TestBIFFHealthyFile(t *testing.T) {
	data, err := os.ReadFile(`../../../test/corpus/files/vendor-40.xls`)
	if err != nil {
		t.Skipf("corpus file missing: %v", err)
	}
	rows, err := ReadRows(data, "vendor-40.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(rows) < 1135 {
		t.Fatalf("rows = %d, want >= 1135", len(rows))
	}
	want := []string{"سعر الجمهور", "الخصم", "اسم الصنف", "كود الصنف"}
	for c, w := range want {
		if g := CleanCell(rows[0][c]); g != w {
			t.Errorf("header col %d: got %q want %q", c, g, w)
		}
	}
	first := []string{"130", "25", "اب دات كريم تفتيح البشرة 50 جرام", "23350"}
	for c, w := range first {
		if g := CleanCell(rows[1][c]); g != w {
			t.Errorf("row 1 col %d: got %q want %q", c, g, w)
		}
	}
}

func TestCorruptedXLS_NeverPanics(t *testing.T) {
	// Corrupted OLE2 file
	ole2Header := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	corrupted := append(ole2Header, []byte("random corrupted trailing sectors and records")...)

	// Open must never panic
	var book *Book
	var err error
	if panics := func() (p bool) {
		defer func() {
			if r := recover(); r != nil {
				p = true
			}
		}()
		book, err = Open(corrupted, "corrupt.xls")
		return false
	}(); panics {
		t.Fatal("sheet.Open panicked on corrupted XLS")
	}

	if err == nil && book != nil {
		_ = book.Close()
	}

	// ReadRows must never panic
	if panics := func() (p bool) {
		defer func() {
			if r := recover(); r != nil {
				p = true
			}
		}()
		_, _ = ReadRows(corrupted, "corrupt.xls")
		return false
	}(); panics {
		t.Fatal("sheet.ReadRows panicked on corrupted XLS")
	}
}
