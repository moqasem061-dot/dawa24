package productmatch

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Sheet layout.
//
// A supplier catalogue is rarely one clean rectangle. Real files arrive with a
// warehouse name and a date on the first line, four blank rows, and the column
// titles on row six. Others reprint the titles every seventy-nine rows because
// the ERP paginates for print — one file carried a hundred and fifteen copies
// of its own header. Others stack two differently shaped sections in one sheet.
//
// So the sheet is read as a header plus a stream of rows, and the stream itself
// carries the rules for recognising a reprinted or replaced header as it goes.
// Nothing needs the whole grid in memory to work this out.

// headerScanRows is how far down the sheet the title row is looked for. A row
// of labels appearing below this is a reprint, which the walk handles, not the
// first header.
const headerScanRows = 40

// minSectionScore is what a row must score to replace the header mid-file on
// its own evidence. One confident binding scores 100, so this demands roughly
// three — enough that a product legitimately named "سعر الصنف" cannot split a
// sheet in half.
const minSectionScore = 220

// Layout is the structure found in a sheet's head.
type Layout struct {
	// HeaderRow is the zero-based row holding the column titles, or -1 when the
	// file has none and the columns are identified from their values alone.
	HeaderRow int `json:"header_row"`
	// FirstDataRow is where the products start.
	FirstDataRow int `json:"first_data_row"`
	// Headers are the title cells, padded to Width. For a headerless file they
	// are generated placeholders so every column still has something to call
	// itself on the review screen.
	Headers []string `json:"headers"`
	Width   int      `json:"width"`
	// TitleRows are the rows above the header that held something — a warehouse
	// name, an export date. They are reported so a vendor can see the analyser
	// understood they were not data.
	TitleRows []int `json:"title_rows,omitempty"`
	// Headerless is true when no row named any known field.
	Headerless bool `json:"headerless"`
	// Score is the winning row's header score, kept for the explanation shown
	// beside the detected structure.
	Score int `json:"score"`
}

// Note describes something about the structure worth telling the vendor.
type Note struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Severity ranks a finding.
type Severity string

const (
	// SeverityError blocks: the affected row or the whole file is not imported.
	SeverityError Severity = "error"
	// SeverityWarning means it was imported with an assumption applied.
	SeverityWarning Severity = "warning"
	// SeverityInfo is an observation with no consequence.
	SeverityInfo Severity = "info"
)

// AnalyzeLayout finds the header row in a sampled sheet head.
func AnalyzeLayout(rows [][]string) (Layout, []Note) {
	layout := Layout{HeaderRow: -1, FirstDataRow: 0}
	if len(rows) == 0 {
		return layout, nil
	}
	for _, row := range rows {
		if len(row) > layout.Width {
			layout.Width = len(row)
		}
	}

	best, bestScore := -1, 0
	limit := min(headerScanRows, len(rows))
	for i := 0; i < limit; i++ {
		if !couldBeHeader(rows[i]) {
			continue
		}
		// Later rows scoring the same are usually the first data row echoing the
		// header's vocabulary, so earlier candidates win ties.
		score := scoreHeaderRow(rows[i]) - i
		if score > bestScore {
			best, bestScore = i, score
		}
	}

	var notes []Note
	if best < 0 {
		layout.Headerless = true
		layout.Headers = placeholderHeaders(layout.Width)
		notes = append(notes, Note{
			Severity: SeverityWarning,
			Message: "لم يتم العثور على صف عناوين في الملف. تم التعرف على الأعمدة من محتوى " +
				"الصفوف نفسها، ويجب مراجعة الربط بعناية قبل المتابعة.",
		})
		return layout, notes
	}

	layout.HeaderRow = best
	layout.FirstDataRow = best + 1
	layout.Score = bestScore
	layout.Headers = padRow(rows[best], layout.Width)
	for i := 0; i < best; i++ {
		if !isBlankRow(rows[i]) {
			layout.TitleRows = append(layout.TitleRows, i)
		}
	}
	if len(layout.TitleRows) > 0 {
		notes = append(notes, Note{
			Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"يبدأ الجدول من الصف %d؛ تم تجاهل %d صفاً أعلاه (عنوان التقرير أو تاريخ التصدير).",
				best+1, len(layout.TitleRows)),
		})
	}
	if blanks := countBlankHeaders(layout.Headers); blanks > 0 {
		notes = append(notes, Note{
			Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"يحتوي صف العناوين على %d عمود بدون عنوان؛ تم التعرف عليها من محتواها.", blanks),
		})
	}
	return layout, notes
}

// couldBeHeader is the coarse filter in front of the expensive scoring.
//
// Column titles are words, not values: a row carrying a purely numeric cell is
// data, and a row with fewer than two labels is too thin to be a header. Both
// tests are a few string scans, against a full evaluation for the rows that
// survive.
func couldBeHeader(row []string) bool {
	filled := 0
	for _, cell := range row {
		clean := sheet.CleanCell(cell)
		if clean == "" {
			continue
		}
		if _, err := sheet.Coerce(clean); err == nil {
			return false
		}
		filled++
	}
	return filled >= 2
}

// scoreHeaderRow rates how much a row looks like a set of column titles.
//
// It rewards cells that name a known field and penalises what marks a row as
// data: repeated values, and cells long enough to be product names. A header
// names things; a data row states them.
func scoreHeaderRow(row []string) int {
	score, filled, duplicates := 0, 0, 0
	seen := map[string]bool{}
	for _, cell := range row {
		clean := sheet.CleanCell(cell)
		if clean == "" {
			continue
		}
		filled++
		key := sheet.NormalizeKey(clean)
		if key != "" && seen[key] {
			duplicates++
		}
		seen[key] = true

		best := 0
		for _, spec := range foldedSpecs {
			if s, blocked := scoreHeader(spec, key); !blocked && s > best {
				best = s
			}
		}
		if best >= scoreFloor {
			score += best
		}
		// A title is a label. Twenty-five characters is a product name.
		if len([]rune(clean)) > 25 {
			score -= 40
		}
	}
	if filled == 0 {
		return 0
	}
	return score - duplicates*40
}

// HeaderGuard recognises, row by row, the header rows that reappear inside the
// data — and the rows that replace it with a differently shaped section.
//
// It exists so the import can stream. The alternative is scanning the whole
// sheet twice, once to find every header and once to read the rows between
// them, which for a nine-thousand-row workbook is the difference between one
// pass and two.
type HeaderGuard struct {
	keys map[string]bool
}

// NewHeaderGuard prepares the guard from the detected header row.
func NewHeaderGuard(headers []string) *HeaderGuard {
	keys := make(map[string]bool, len(headers))
	for _, cell := range headers {
		if key := sheet.NormalizeKey(cell); key != "" {
			keys[key] = true
		}
	}
	return &HeaderGuard{keys: keys}
}

// RowKind is what a streamed row turned out to be.
type RowKind int

const (
	// RowData is a product row.
	RowData RowKind = iota
	// RowBlank is empty and is counted, not reported.
	RowBlank
	// RowRepeatedHeader is the detected header printed again by a paginated
	// export. It is skipped.
	RowRepeatedHeader
	// RowSectionHeader is a new set of column titles, differently shaped from
	// the first. The caller re-resolves the mapping from it.
	RowSectionHeader
)

// Classify decides what one streamed row is.
func (g *HeaderGuard) Classify(row []string) RowKind {
	if isBlankRow(row) {
		return RowBlank
	}
	if g.isRepeat(row) {
		return RowRepeatedHeader
	}
	if couldBeHeader(row) && scoreHeaderRow(row) >= minSectionScore {
		return RowSectionHeader
	}
	return RowData
}

// isRepeat reports whether a data row is really the header printed again.
//
// Distributor systems paginate their exports and reprint the column titles
// every page. Matching against the detected header generalises past a hardcoded
// list of literal strings, which would silently import every reprinted header
// of every file whose titles were not on that list as if it were a product.
func (g *HeaderGuard) isRepeat(row []string) bool {
	if len(g.keys) == 0 {
		return false
	}
	filled, matched := 0, 0
	for _, cell := range row {
		key := sheet.NormalizeKey(cell)
		if key == "" {
			continue
		}
		filled++
		if g.keys[key] {
			matched++
		}
	}
	// Two-thirds agreement over at least two cells, so a product legitimately
	// named after a column cannot trip it on its own.
	return filled >= 2 && matched*3 >= filled*2
}

// Adopt replaces the guard's reference header after a section break, so the new
// section's own reprints are recognised too.
func (g *HeaderGuard) Adopt(headers []string) {
	g.keys = NewHeaderGuard(headers).keys
}

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if sheet.CleanCell(cell) != "" {
			return false
		}
	}
	return true
}

func padRow(row []string, width int) []string {
	out := make([]string, width)
	for i := 0; i < width && i < len(row); i++ {
		out[i] = sheet.CleanCell(row[i])
	}
	return out
}

func countBlankHeaders(headers []string) int {
	n := 0
	for _, h := range headers {
		if h == "" {
			n++
		}
	}
	return n
}

// placeholderHeaders names the columns of a headerless file, so the review
// screen and every warning have something to point at.
func placeholderHeaders(width int) []string {
	out := make([]string, width)
	for i := range out {
		out[i] = fmt.Sprintf("العمود %d", i+1)
	}
	return out
}
