package sheet

// Reading a whole file into a grid.
//
// Streaming is the right shape for an import: a hundred-thousand-row workbook
// costs one row of memory. It is the wrong shape for a wizard that has to show
// the user a preview, let them re-map a column, and read the same file again —
// there the file is small, the code is a handler, and a grid is what it wants.
//
// So this is offered alongside Walk rather than instead of it. What it replaces
// is internal/shared/spreadsheet, a second reader that existed only to provide
// this one function and that had drifted: it sniffed formats the streaming
// reader did not and missed formats the streaming reader handled, so two
// features could disagree about whether the same upload was readable.

import (
	"errors"
	"fmt"
)

// maxGridRows bounds what a caller may pull into memory at once.
//
// Two hundred thousand rows of a dozen columns is roughly a hundred megabytes
// of Go strings — already past what a request handler should hold, and far past
// any file a person is going to review by hand in a wizard. A caller that
// genuinely needs more is doing an import, and an import streams.
const maxGridRows = 200_000

// ErrTooLarge is returned for a file with more rows than a grid should hold.
var ErrTooLarge = errors.New("sheet: file too large to read whole; stream it instead")

// ReadRows decodes an entire file into a grid of cleaned cells.
//
// Every cell is passed through CleanCell, which strips the NUL and C0 padding
// that legacy BIFF writes inside its string records. PostgreSQL rejects those
// outright in a text column, so a single invisible byte used to fail a whole
// import batch with an error naming an encoding rather than a row.
//
// filename only improves error messages and delimiter hints; the format is
// decided by the bytes.
func ReadRows(content []byte, filename string, opts ...OpenOption) (rows [][]string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة بيانات الملف — قد يكون الملف تالفاً (%v)", r)
			rows = nil
		}
	}()

	book, err := Open(content, filename, opts...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = book.Close() }()

	grid := make([][]string, 0, 512)
	err = book.Walk(func(_ int, row []string) error {
		if len(grid) >= maxGridRows {
			return ErrTooLarge
		}
		out := make([]string, len(row))
		for i, cell := range row {
			out[i] = CleanCell(cell)
		}
		grid = append(grid, out)
		return nil
	})
	if err != nil && !errors.Is(err, ErrStop) {
		return nil, err
	}
	if len(grid) == 0 {
		return nil, ErrEmpty
	}

	// The OOXML decoder trims trailing empty cells, so a row whose last columns
	// are blank comes back short while the whole-decoded formats come back
	// padded. A caller indexing by column must not have to know which decoder
	// ran, so every row is widened to the sheet's width here.
	width := 0
	for _, row := range grid {
		if len(row) > width {
			width = len(row)
		}
	}
	pad(grid, width)
	return grid, nil
}
