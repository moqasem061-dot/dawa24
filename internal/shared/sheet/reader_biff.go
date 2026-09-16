package sheet

import (
	"bytes"
	"fmt"

	xreader "github.com/shakinm/xlsReader/xls"
)

// openXLSModern decodes a legacy Excel 97-2003 workbook with a BIFF reader
// that reassembles SST Continue records correctly.
//
// The previous decoder (extrame/xls, see openXLSLegacy in reader_text.go)
// mis-handles Continue records past a certain offset in real distributor
// files: from that row on it returns one cell holding the undecoded blob of
// hundreds of rows glued together, and every row after it reads back empty.
// The corruptBIFF guard then refuses the whole file, so the compare tool
// could not map, save, or match those .xls files at all — while Excel itself
// opens them perfectly.
//
// This path runs first. Anything it cannot decode falls through to the legacy
// path, so files the old decoder already handled keep working exactly as
// before.
func (b *Book) openXLSModern() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("modern XLS decode panicked: %v", r)
		}
	}()

	wb, err := xreader.OpenReader(bytes.NewReader(b.content))
	if err != nil {
		return fmt.Errorf("modern XLS open: %w", err)
	}

	n := wb.GetNumberSheets()
	if n <= 0 {
		return fmt.Errorf("modern XLS: no worksheets")
	}

	type decodedSheet struct {
		name string
		grid [][]string
	}
	sheets := make([]decodedSheet, 0, n)
	for i := 0; i < n; i++ {
		sh, serr := wb.GetSheet(i)
		if serr != nil || sh == nil {
			continue
		}
		name := sh.GetName()
		var grid [][]string
		total := sh.GetNumberRows()
		for r := 0; r <= total; r++ {
			row, rerr := sh.GetRow(r)
			if rerr != nil || row == nil {
				grid = append(grid, nil)
				continue
			}
			cols := row.GetCols()
			if len(cols) == 0 {
				grid = append(grid, nil)
				continue
			}
			cells := make([]string, len(cols))
			for c, cell := range cols {
				if cell == nil {
					continue
				}
				// Number cells render via FormatFloat(f,-1) inside the
				// library; text cells come back decoded. Either way the
				// downstream CleanCell/Coerce stages see faithful strings.
				cells[c] = cell.GetString()
			}
			grid = append(grid, cells)
		}
		sheets = append(sheets, decodedSheet{name: name, grid: grid})
	}
	if len(sheets) == 0 {
		return fmt.Errorf("modern XLS: no readable worksheets")
	}

	// Densest sheet wins, same rule as the legacy path.
	best, bestCells := -1, 0
	grids := make([][][]string, len(sheets))
	for i, sh := range sheets {
		grids[i] = sh.grid
		info := SheetInfo{Name: sh.name, Rows: len(sh.grid)}
		for _, row := range sh.grid {
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
		if info.Cells > bestCells {
			bestCells, best = info.Cells, len(b.source.Sheets)-1
		}
	}
	if best < 0 || bestCells == 0 {
		return fmt.Errorf("modern XLS: no data cells")
	}
	if corruptBIFF(grids[best]) {
		return fmt.Errorf("modern XLS: decoded grid failed integrity check")
	}

	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.rows = grids[best]
	b.finishGrid()
	return nil
}
