package sheet

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// OOXML workbook decoding.
//
// excelize can hand back a whole worksheet as [][]string, and that is what
// every importer here used to do. It is also why a 9,000-row supplier export
// held three copies of itself in memory during an import — the raw grid, the
// parsed rows, and the staged products. The row iterator streams the same XML
// for the same result at one row of cost, so it is used for both the sample and
// the import.

// sheetProbeRows is how deep into each worksheet the chooser looks when
// deciding which tab holds the catalogue. Density over a few hundred rows
// separates a data sheet from a cover page without parsing either in full.
const sheetProbeRows = 200

type xlsxBook struct {
	f     *excelize.File
	sheet string
}

func (x *xlsxBook) close() error {
	if x.f == nil {
		return nil
	}
	return x.f.Close()
}

// openXLSX reads the workbook index and picks the sheet holding the catalogue.
//
// "The sheet with the most rows" is not good enough: exports routinely carry a
// trailing tab of a thousand blank formatted rows, and a summary tab whose
// twenty rows are the real data. Density decides instead, and hidden sheets are
// skipped because a hidden sheet is something the supplier set aside.
func (b *Book) openXLSX() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة ملف Excel (.xlsx) — قد يكون الملف تالفاً (%v)", r)
		}
	}()

	f, err := excelize.OpenReader(bytes.NewReader(b.content))
	if err != nil {
		return fmt.Errorf("تعذر فتح ملف Excel — قد يكون الملف تالفاً أو محمياً بكلمة مرور (%v)", err)
	}

	names := f.GetSheetList()
	if len(names) == 0 {
		_ = f.Close()
		return errors.New("ملف Excel لا يحتوي على أي أوراق عمل")
	}

	best, bestScore := -1, 0
	for _, name := range names {
		info := SheetInfo{Name: name}
		if visible, vErr := f.GetSheetVisible(name); vErr == nil && !visible {
			info.Hidden = true
		}
		info.Cells, info.Width, info.Rows = probeSheet(f, name)
		if dim, dErr := f.GetSheetDimension(name); dErr == nil {
			if n := parseDimensionRows(dim); n > info.Rows {
				info.Rows = n
			}
		}
		b.source.Sheets = append(b.source.Sheets, info)

		if info.Hidden || info.Cells == 0 {
			continue
		}
		// Approximate the sheet's total cell count: density in the sampled head
		// carried across its declared extent. That is what separates a wide but
		// short cover page from a narrow nine-thousand-row price list.
		sampled := min(info.Rows, sheetProbeRows)
		score := info.Cells
		if sampled > 0 && info.Rows > sampled {
			score = info.Cells * info.Rows / sampled
		}
		if score > bestScore {
			bestScore, best = score, len(b.source.Sheets)-1
		}
	}

	if best < 0 {
		_ = f.Close()
		return errors.New("جميع أوراق العمل في ملف Excel فارغة. يرجى التأكد من حفظ البيانات في الملف قبل رفعه")
	}

	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.source.TotalRows = b.source.Sheets[best].Rows
	b.source.Estimated = true
	b.xlsx = &xlsxBook{f: f, sheet: b.source.Sheet}
	return nil
}

// probeSheet counts non-empty cells over the head of a worksheet.
func probeSheet(f *excelize.File, name string) (cells, width, seen int) {
	defer func() {
		if r := recover(); r != nil {
			// keep whatever was counted
		}
	}()

	rows, err := f.Rows(name)
	if err != nil {
		return 0, 0, 0
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		seen++
		cols, cErr := rows.Columns()
		if cErr != nil {
			break
		}
		if len(cols) > width {
			width = len(cols)
		}
		for _, cell := range cols {
			if CleanCell(cell) != "" {
				cells++
			}
		}
		if seen >= sheetProbeRows {
			break
		}
	}
	return cells, width, seen
}

// walk streams the chosen worksheet.
//
// The iterator's own cursor is the spreadsheet row number, so blank rows arrive
// as empty slices at their real position rather than being silently closed up.
// Every issue this import reports can therefore name a row the vendor can find
// in their own copy of the file.
func (x *xlsxBook) walk(fn RowFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة صفوف ملف Excel (.xlsx) — قد يكون الملف تالفاً (%v)", r)
		}
	}()

	rows, err := x.f.Rows(x.sheet)
	if err != nil {
		return fmt.Errorf("تعذر قراءة ورقة العمل «%s»: %w", x.sheet, err)
	}
	defer func() { _ = rows.Close() }()

	index := 0
	for rows.Next() {
		cols, cErr := rows.Columns()
		if cErr != nil {
			return fmt.Errorf("تعذر قراءة الصف %d من ورقة العمل «%s»: %w", index+1, x.sheet, cErr)
		}
		if err := fn(index, cols); err != nil {
			return err
		}
		index++
	}
	return rows.Error()
}

// parseDimensionRows reads the trailing row number out of an OOXML dimension
// reference such as "A1:H9020".
func parseDimensionRows(dim string) int {
	_, end, ok := strings.Cut(dim, ":")
	if !ok {
		return 0
	}
	digits := strings.TrimLeftFunc(end, func(r rune) bool { return r < '0' || r > '9' })
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}
