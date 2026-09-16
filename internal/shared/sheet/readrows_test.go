package sheet_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// These carry over the coverage of internal/shared/spreadsheet, the second
// reader this package replaced. Each case is a format an Egyptian distributor
// has actually sent, and the XML Spreadsheet one is new: neither reader parsed
// it correctly before, because it sniffs as HTML and an HTML parser finds a
// table with no rows in it.

func TestReadRowsXLSX(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	rows := [][]any{
		{"اسم الصنف", "كود الصنف", "سعر الجمهور"},
		{"بانادول اكسترا 24 قرص", "PAN-24", 48.5},
		{"كونجستال 20 قرص", "CON-20", 29.0},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellValue("Sheet1", cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := sheet.ReadRows(buf.Bytes(), "prices.xlsx")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3", len(got))
	}
	if got[1][0] != "بانادول اكسترا 24 قرص" {
		t.Errorf("cell = %q", got[1][0])
	}
}

func TestReadRowsCSVWithBOMAndSemicolons(t *testing.T) {
	// A BOM and semicolons together are what Excel writes on an Arabic Windows
	// locale, and the pair defeated the previous reader's comma-only split.
	csv := "\xEF\xBB\xBFاسم الصنف;الكمية;السعر\nبانادول;10;48,50\n"

	got, err := sheet.ReadRows([]byte(csv), "list.csv")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2: %#v", len(got), got)
	}
	if got[0][0] != "اسم الصنف" {
		t.Errorf("BOM not stripped: %q", got[0][0])
	}
	if len(got[1]) != 3 || got[1][1] != "10" {
		t.Errorf("row = %#v", got[1])
	}
}

func TestReadRowsHTMLTable(t *testing.T) {
	doc := `<html><body><table>
		<tr><th>Name</th><th>Price</th></tr>
		<tr><td>Panadol</td><td>48.50</td></tr>
	</table></body></html>`

	got, err := sheet.ReadRows([]byte(doc), "export.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(got) != 2 || got[1][0] != "Panadol" {
		t.Fatalf("got %#v", got)
	}
}

// TestReadRowsXMLSpreadsheet2003 covers the format older Egyptian ERPs export
// under a .xls name. The blank second column is stated with ss:Index rather
// than written out, which is the part that silently shifts every later value
// one column left when it is ignored.
func TestReadRowsXMLSpreadsheet2003(t *testing.T) {
	doc := `<?xml version="1.0"?>
<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet"
          xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">
 <Worksheet ss:Name="Prices">
  <Table>
   <Row>
    <Cell><Data ss:Type="String">اسم الصنف</Data></Cell>
    <Cell><Data ss:Type="String">الكود</Data></Cell>
    <Cell><Data ss:Type="String">السعر</Data></Cell>
   </Row>
   <Row>
    <Cell><Data ss:Type="String">بانادول اكسترا</Data></Cell>
    <Cell ss:Index="3"><Data ss:Type="Number">48.5</Data></Cell>
   </Row>
  </Table>
 </Worksheet>
</Workbook>`

	if f := sheet.Detect([]byte(doc)); f != sheet.FormatXML2003 {
		t.Fatalf("Detect = %q, want xml2003", f)
	}

	got, err := sheet.ReadRows([]byte(doc), "prices.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2: %#v", len(got), got)
	}
	if got[0][0] != "اسم الصنف" {
		t.Errorf("header = %q", got[0][0])
	}
	// The price must land in column 3, not column 2.
	if len(got[1]) < 3 {
		t.Fatalf("row too short: %#v", got[1])
	}
	if got[1][1] != "" {
		t.Errorf("column 2 = %q, want empty (ss:Index skipped it)", got[1][1])
	}
	if got[1][2] != "48.5" {
		t.Errorf("price landed at %#v", got[1])
	}
}

// TestReadRowsScrubsControlBytes guards the failure that used to abort a whole
// import: legacy BIFF pads its string records with NUL, PostgreSQL refuses NUL
// in a text column, and the error named an encoding rather than a row.
func TestReadRowsScrubsControlBytes(t *testing.T) {
	csv := "name,code\nPanadol\x00,PAN\x01-1\n"

	got, err := sheet.ReadRows([]byte(csv), "dirty.csv")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	joined := strings.Join(got[1], "|")
	if strings.ContainsAny(joined, "\x00\x01") {
		t.Fatalf("control bytes survived: %q", joined)
	}
	if got[1][0] != "Panadol" {
		t.Errorf("cell = %q", got[1][0])
	}
}

func TestReadRowsRejectsEmptyFile(t *testing.T) {
	if _, err := sheet.ReadRows(nil, "empty.csv"); err == nil {
		t.Fatal("expected an error for an empty file")
	}
}

func TestReadRowsRejectsSuspiciousURLsAndDomains(t *testing.T) {
	badCSV := "name,price,website\nPanadol,50.00,http://evil.com\n"
	_, err := sheet.ReadRows([]byte(badCSV), "bad.csv")
	if err == nil || err.Error() != "فشل الرفع لأسباب امنية" {
		t.Fatalf("expected 'فشل الرفع لأسباب امنية', got: %v", err)
	}

	cleanCSV := "name,price,form\nPanadol,50.00,tablet\n"
	rows, err := sheet.ReadRows([]byte(cleanCSV), "clean.csv")
	if err != nil {
		t.Fatalf("unexpected error for clean csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got: %d", len(rows))
	}
}
