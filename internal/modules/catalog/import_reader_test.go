package catalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
)

func TestReadSpreadsheetTolerantesRaggedCSV(t *testing.T) {
	// encoding/csv locks the field count to the first record unless told
	// otherwise, and the importer used to let it: one short row failed the whole
	// upload with "record on line 3: wrong number of fields", discarding
	// thousands of good rows with it.
	csv := "اسم الصنف,كود الصنف,السعر\n" +
		"بانادول,PAN-1,25.00\n" +
		"كتافلام,PAN-2\n" +
		"اوجمنتين,PAN-3,115.00,ملاحظة إضافية\n"

	data, err := catalog.ReadSpreadsheet([]byte(csv), "list.csv")
	if err != nil {
		t.Fatalf("ragged CSV rejected: %v", err)
	}
	if len(data.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(data.Rows))
	}
	// Every row is padded to the widest, so a mapped column index is always
	// addressable without a bounds check at every read site.
	for i, row := range data.Rows {
		if len(row) != data.Width {
			t.Errorf("row %d has width %d, want %d", i, len(row), data.Width)
		}
	}
}

func TestReadSpreadsheetSniffsDelimiters(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		columns int
	}{
		{"comma", "a,b,c\n1,2,3\n4,5,6\n", ",", 3},
		{"semicolon", "a;b;c\n1;2;3\n4;5;6\n", ";", 3},
		{"tab", "a\tb\tc\n1\t2\t3\n4\t5\t6\n", "\t", 3},
		{"pipe", "a|b|c\n1|2|3\n4|5|6\n", "|", 3},
		// A comma inside a quoted product name must not outvote the real
		// delimiter — counting the header line alone got this wrong.
		{"quoted comma with semicolons", "\"اسم, تجاري\";كود;سعر\n\"بانادول, أقراص\";P1;25\n\"كتافلام, فوار\";P2;42\n", ";", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := catalog.ReadSpreadsheet([]byte(tc.content), "f.csv")
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if data.Delimiter != tc.want {
				t.Errorf("delimiter = %q, want %q", data.Delimiter, tc.want)
			}
			if data.Width != tc.columns {
				t.Errorf("width = %d, want %d", data.Width, tc.columns)
			}
		})
	}
}

func TestReadSpreadsheetStripsBOM(t *testing.T) {
	data, err := catalog.ReadSpreadsheet([]byte("\xEF\xBB\xBFاسم الصنف,السعر\nبانادول,25\n"), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got := data.Rows[0][0]; got != "اسم الصنف" {
		t.Errorf("BOM survived into first header cell: %q", got)
	}
}

func TestReadSpreadsheetRejectsUnusableFiles(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wants   string
	}{
		{"empty", nil, "فارغ"},
		{"legacy xls", append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...), ".xlsx"},
		{"html", []byte("<html><body><table></table></body></html>"), "HTML"},
		{"pdf", []byte("%PDF-1.7\nrubbish"), "PDF"},
		{"binary", append([]byte("MZ\x90\x00"), make([]byte, 600)...), "تعذر التعرف"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := catalog.ReadSpreadsheet(tc.content, "upload.dat")
			if err == nil {
				t.Fatal("expected a refusal, got none")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("message %q does not mention %q", err.Error(), tc.wants)
			}
		})
	}
}

func TestReadSpreadsheetReadsLegacyXLS(t *testing.T) {
	// A real Excel 97-2003 workbook. Roughly a fifth of what Egyptian
	// distributors send is one of these, and this importer used to refuse them
	// outright while the vendor import — reading the same file through the
	// shared decoder — accepted them. The same file must not be importable
	// through one screen and rejected by another.
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "corpus", "files", "vendor-40.xls"))
	if err != nil {
		t.Skip("corpus not exported; run `go run ./cmd/cli corpus-export`")
	}
	data, err := catalog.ReadSpreadsheet(content, "old.xls")
	if err != nil {
		t.Fatalf("legacy .xls refused: %v", err)
	}
	if len(data.Rows) == 0 {
		t.Fatal("legacy .xls decoded to no rows")
	}
	if data.Format != "xls" {
		t.Errorf("format reported as %q, want \"xls\"", data.Format)
	}
}

func TestReadSpreadsheetRefusesUnreadableXLS(t *testing.T) {
	// A file claiming to be BIFF and carrying nothing decodable. The refusal
	// must name the actual problem rather than the format.
	_, err := catalog.ReadSpreadsheet(
		append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...), "old.xls")
	if err == nil {
		t.Fatal("a corrupt .xls was accepted")
	}
}

func TestReadSpreadsheetDecodesUTF16(t *testing.T) {
	// Excel's "Unicode Text" export. Read as UTF-8 it is one column of mojibake.
	utf16le := []byte{0xFF, 0xFE}
	for _, r := range "اسم,سعر\nبانادول,25\n" {
		utf16le = append(utf16le, byte(r), byte(r>>8))
	}

	data, err := catalog.ReadSpreadsheet(utf16le, "unicode.txt")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if data.Rows[0][0] != "اسم" {
		t.Errorf("UTF-16 not decoded: %q", data.Rows[0][0])
	}
}

func TestReadSpreadsheetKeepsBlankLinesAtTheirRowNumbers(t *testing.T) {
	// encoding/csv drops blank lines, which silently shifts every later row
	// number away from the gutter in the admin's own spreadsheet — and the
	// import report promises those numbers match.
	csv := "اسم الصنف,السعر\n\n\nبانادول,25.00\n"

	data, err := catalog.ReadSpreadsheet([]byte(csv), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data.Rows) != 4 {
		t.Fatalf("got %d rows, want 4 with the two blanks preserved", len(data.Rows))
	}
	if got := catalog.CleanCellString(data.Rows[3][0]); got != "بانادول" {
		t.Errorf("row 4 holds %q, want بانادول — the product moved up by the dropped blanks", got)
	}
}

func TestReadSpreadsheetAllowsImageURLs(t *testing.T) {
	csv := "اسم الصنف,كود الصنف,السعر,رابط الصورة\n" +
		"بانادول اكسترا,PAN-1,25.00,https://cdn.example.com/products/panadol.jpg\n"

	data, err := catalog.ReadSpreadsheet([]byte(csv), "catalog_with_images.csv", filesecurity.WithAllowURLs(true))
	if err != nil {
		t.Fatalf("ReadSpreadsheet failed to read file with image URLs: %v", err)
	}
	if len(data.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(data.Rows))
	}
	if got := catalog.CleanCellString(data.Rows[1][3]); got != "https://cdn.example.com/products/panadol.jpg" {
		t.Errorf("image url = %q, want https://cdn.example.com/products/panadol.jpg", got)
	}
}

