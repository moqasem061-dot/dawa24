package sheet

// Reading an Excel 97-2003 workbook without a library.
//
// This package already had two BIFF decoders and a real distributor file
// defeated both. Ø§Ù„Ø¯Ù‚Ù‡Ù„ÙŠÙ‡-1.XLS holds 764 rows in four columns; its record
// stream holds 767 shared strings and 767 LABELSST cells, so every product name
// is in the file and countable. github.com/shakinm/xlsReader panics on it
// ("slice bounds out of range") and github.com/extrame/xls returns 155 of the
// 767 strings and empty cells for the rest â€” and the import that followed said,
// truthfully as far as it could see, that 609 of the vendor's 764 products had
// no name and could not be matched. That is the screen this file was written
// for.
//
// What both libraries get wrong is the same thing: the shared-string table is
// larger than one BIFF record, so it continues into CONTINUE records, and a
// string may be cut in half at that boundary. The continuation then begins with
// a fresh encoding byte describing the REMAINDER of that string â€” the one rule
// in the format that cannot be discovered by reading a well-formed small file,
// and the one that decides whether the second half of a price list is readable.
//
// So the decoder here is deliberately narrow. It reads BIFF8, it reads the
// records that carry values, and it hands the grid to the same finishGrid the
// other readers use. It does not write, does not evaluate formulas, and does
// not format numbers: a date reaches the pipeline as its serial, which
// CoerceDate already reads.

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// BIFF record numbers, in the order this file uses them.
const (
	recFormula    = 0x0006
	recEOF        = 0x000A
	recFilePass   = 0x002F
	recContinue   = 0x003C
	recBoundSheet = 0x0085
	recMulRK      = 0x00BD
	recMulBlank   = 0x00BE
	recRString    = 0x00D6
	recSST        = 0x00FC
	recLabelSST   = 0x00FD
	recBlank      = 0x0201
	recNumber     = 0x0203
	recLabel      = 0x0204
	recBoolErr    = 0x0205
	recString     = 0x0207
	recRK         = 0x027E
	recBOF        = 0x0809
)

// biffVersion8 is the only version this decoder reads. Earlier workbooks have a
// different string encoding and no shared-string table, and the existing
// decoders handle them; a file this one refuses simply falls through to them.
const biffVersion8 = 0x0600

// Bounds on what a workbook may claim, so a malformed file cannot make the
// importer allocate without limit.
const (
	maxBiffRows   = 1 << 20
	maxBiffCols   = 1 << 14
	maxSSTStrings = 1 << 21
)

// biffRecord is one record's number and body, with CONTINUE records kept
// separate rather than glued on: whether a byte sits at a record boundary is
// what the shared-string reader has to know.
type biffRecord struct {
	id   uint16
	body []byte
	// continues are the CONTINUE records that followed this one, in order.
	continues [][]byte
	// offset is where this record began in the workbook stream, which is how a
	// BOUNDSHEET points at its worksheet.
	offset int
}

// openXLSNative decodes a legacy workbook with this package's own BIFF reader.
func (b *Book) openXLSNative() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("native XLS decode panicked: %v", r)
		}
	}()

	container, err := openCompoundFile(b.content)
	if err != nil {
		return err
	}
	stream, err := workbookStream(container)
	if err != nil {
		return err
	}
	records, err := splitBiffRecords(stream)
	if err != nil {
		return err
	}

	globals, err := globalsOf(records)
	if err != nil {
		return err
	}
	if len(globals.sheets) == 0 {
		return errors.New("native XLS: workbook declares no worksheets")
	}
	// A workbook with no shared strings is one whose cells are all numeric, and
	// it decodes perfectly well with an empty table.
	var sst []string
	if globals.sst >= 0 {
		sst = readSharedStrings(records[globals.sst])
	}

	best, bestCells := -1, 0
	grids := make([][][]string, 0, len(globals.sheets))
	for _, sh := range globals.sheets {
		grid := decodeSheet(records, stream, sh.offset, sst)
		grids = append(grids, grid)

		info := SheetInfo{Name: sh.name, Rows: len(grid), Hidden: sh.hidden}
		for _, row := range grid {
			if len(row) > info.Width {
				info.Width = len(row)
			}
			for _, cell := range row {
				if CleanCell(cell) != "" {
					info.Cells++
				}
			}
		}
		b.source.Sheets = append(b.source.Sheets, info)
		if !info.Hidden && info.Cells > bestCells {
			bestCells, best = info.Cells, len(b.source.Sheets)-1
		}
	}
	if best < 0 || bestCells == 0 {
		return errors.New("native XLS: no data cells")
	}

	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.rows = grids[best]
	b.finishGrid()
	return nil
}

// workbookStream returns the BIFF record stream, under either name Excel has
// used for it.
func workbookStream(c *compoundFile) ([]byte, error) {
	stream, err := c.stream("Workbook")
	if err == nil {
		return stream, nil
	}
	return c.stream("Book")
}

// splitBiffRecords walks the stream into records, attaching each CONTINUE to
// the record it continues.
//
// A record whose declared length runs past the end of the stream ends the walk
// rather than failing it. Real workbooks carry trailing structures this decoder
// does not read â€” the drawing layer, in the corpus file â€” and stopping there
// with every cell already collected is the correct outcome, where refusing the
// file would throw away the data over bytes nothing needed.
func splitBiffRecords(stream []byte) ([]biffRecord, error) {
	var out []biffRecord
	for i := 0; i+4 <= len(stream); {
		id := binary.LittleEndian.Uint16(stream[i : i+2])
		size := int(binary.LittleEndian.Uint16(stream[i+2 : i+4]))
		if i+4+size > len(stream) {
			break
		}
		body := stream[i+4 : i+4+size]
		if id == recContinue && len(out) > 0 {
			last := &out[len(out)-1]
			last.continues = append(last.continues, body)
		} else {
			out = append(out, biffRecord{id: id, body: body, offset: i})
		}
		i += 4 + size
	}
	if len(out) == 0 {
		return nil, errors.New("native XLS: no readable records")
	}
	return out, nil
}

// worksheetRef is one worksheet named by the workbook globals.
type worksheetRef struct {
	name   string
	offset int
	hidden bool
}

// workbookGlobals is what the first substream declares: where the worksheets
// are and where the shared strings live.
type workbookGlobals struct {
	sheets []worksheetRef
	// sst is the index of the shared-string record, or -1.
	sst int
}

// globalsOf reads the workbook globals substream.
func globalsOf(records []biffRecord) (workbookGlobals, error) {
	g := workbookGlobals{sst: -1}
	if records[0].id != recBOF || len(records[0].body) < 2 ||
		binary.LittleEndian.Uint16(records[0].body[0:2]) < biffVersion8 {
		return g, errors.New("native XLS: not a BIFF8 workbook")
	}
	for i, rec := range records {
		switch rec.id {
		case recFilePass:
			return g, errors.New("native XLS: workbook is password protected")
		case recSST:
			g.sst = i
		case recBoundSheet:
			if sh, ok := readBoundSheet(rec.body); ok {
				g.sheets = append(g.sheets, sh)
			}
		case recEOF:
			if len(g.sheets) > 0 {
				return g, nil
			}
		}
	}
	return g, nil
}

// readBoundSheet reads one worksheet's name, visibility and stream position.
func readBoundSheet(body []byte) (worksheetRef, bool) {
	const worksheetType = 0
	if len(body) < 8 {
		return worksheetRef{}, false
	}
	if body[5] != worksheetType {
		return worksheetRef{}, false // a chart or a macro sheet
	}
	return worksheetRef{
		name:   readShortUnicode(body[6:]),
		offset: int(binary.LittleEndian.Uint32(body[0:4])),
		hidden: body[4]&0x03 != 0,
	}, true
}

// readShortUnicode reads the one-byte-length string BOUNDSHEET carries.
func readShortUnicode(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	return decodeChars(b[2:], int(b[0]), b[1]&sstHighByte != 0)
}

// readLongUnicode reads the two-byte-length string a cell record carries: a
// character count, an options byte, an optional run count, then the characters.
//
// Rich text is read for its characters and nothing else â€” the formatting runs
// that follow describe how Excel painted them, and this importer is reading a
// price list, not rendering one.
func readLongUnicode(b []byte) string {
	if len(b) < 3 {
		return ""
	}
	cch := int(binary.LittleEndian.Uint16(b[0:2]))
	flags := b[2]
	chars := 3
	if flags&sstRichSt != 0 {
		chars += 2
	}
	if flags&sstExtSt != 0 {
		chars += 4
	}
	if chars > len(b) {
		return ""
	}
	return decodeChars(b[chars:], cch, flags&sstHighByte != 0)
}

// decodeChars reads cch characters, from the first character byte, in either of
// BIFF8's two encodings.
func decodeChars(b []byte, cch int, high bool) string {
	if cch <= 0 {
		return ""
	}
	if high {
		n := min(cch*2, len(b)&^1)
		runes := make([]rune, 0, n/2)
		for i := 0; i+1 < n; i += 2 {
			runes = append(runes, rune(binary.LittleEndian.Uint16(b[i:i+2])))
		}
		return string(runes)
	}
	n := min(cch, len(b))
	runes := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		runes = append(runes, rune(b[i]))
	}
	return string(runes)
}

