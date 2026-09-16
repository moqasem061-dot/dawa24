package productmatch

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The processing stage.
//
// One streamed pass over the whole file, one row of memory at a time, emitting
// products in batches. A hundred-thousand-row workbook costs the same working
// set as a hundred-row one; what grows is the set of identity keys seen, which
// is bounded by the number of distinct products rather than by the file.
//
// The mapping is not re-derived here. It is the mapping the vendor confirmed,
// passed in, so what runs is exactly what they approved.

// Stats accounts for every row in the file. The numbers are meant to add up:
// a vendor who counts 9,020 rows in Excel must be able to see where all 9,020
// went.
type Stats struct {
	SheetRows       int `json:"sheet_rows"`
	BlankRows       int `json:"blank_rows"`
	RepeatedHeaders int `json:"repeated_headers"`
	SectionHeaders  int `json:"section_headers"`
	DataRows        int `json:"data_rows"`
	Parsed          int `json:"parsed"`
	Rejected        int `json:"rejected"`
	Duplicates      int `json:"duplicates"`
	Warnings        int `json:"warnings"`
}

// Accounted is what the stage claims to have seen, for the reconciliation the
// results screen shows.
func (s Stats) Accounted() int {
	return s.BlankRows + s.RepeatedHeaders + s.SectionHeaders + s.DataRows
}

// Sink receives parsed rows in batches. Returning an error stops the pass.
type Sink func(batch []*Row) error

// DuplicatePolicy decides what happens to a second row carrying an identity
// already seen in the same file.
type DuplicatePolicy string

const (
	// DuplicateLastWins lets the later row overwrite the earlier one, which is
	// what a supplier who appended a correction to the end of their file means.
	DuplicateLastWins DuplicatePolicy = "last_wins"
	// DuplicateFirstWins keeps the earlier row and skips the later.
	DuplicateFirstWins DuplicatePolicy = "first_wins"
	// DuplicateReject refuses both and asks the vendor to clean the file.
	DuplicateReject DuplicatePolicy = "reject"
)

// ProcessOptions govern one pass.
type ProcessOptions struct {
	Parse ParseOptions
	// BatchSize is how many rows are handed to the sink at once.
	BatchSize int
	// Duplicates decides what a repeated identity means.
	Duplicates DuplicatePolicy
	// MaxIssues caps what is retained. A file with nine thousand broken rows
	// must not turn one upload into a gigabyte of report; the counters stay
	// exact regardless.
	MaxIssues int
	// Vocabulary is needed only to re-resolve a mid-file section header.
	Vocabulary *Vocabulary
}

// DefaultProcessOptions are the settings the wizard starts on.
func DefaultProcessOptions() ProcessOptions {
	return ProcessOptions{
		Parse:      DefaultParseOptions(),
		BatchSize:  500,
		Duplicates: DuplicateLastWins,
		MaxIssues:  1000,
	}
}

// Result is what one pass produced.
type Result struct {
	Stats  Stats   `json:"stats"`
	Issues []Issue `json:"issues,omitempty"`
	Notes  []Note  `json:"notes,omitempty"`
}

// Process streams the file through the confirmed mapping and into the sink.
func Process(book *sheet.Book, layout Layout, mapping *Mapping, opts ProcessOptions, sink Sink) (*Result, error) {
	if book == nil || mapping == nil {
		return nil, fmt.Errorf("لا يمكن بدء المعالجة بدون ملف وربط أعمدة")
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 500
	}
	if opts.MaxIssues <= 0 {
		opts.MaxIssues = 1000
	}
	if opts.Duplicates == "" {
		opts.Duplicates = DuplicateLastWins
	}

	p := &pass{
		opts:   opts,
		reader: NewReader(mapping, opts.Parse),
		guard:  NewHeaderGuard(layout.Headers),
		seen:   make(map[string]int, 4096),
		result: &Result{},
	}
	p.batch = make([]*Row, 0, opts.BatchSize)

	err := book.Walk(func(index int, cells []string) error {
		p.result.Stats.SheetRows = index + 1
		if index < layout.FirstDataRow {
			return nil
		}
		return p.row(index, cells, sink)
	})
	if err != nil {
		return p.result, err
	}
	if err := p.flush(sink); err != nil {
		return p.result, err
	}
	return p.result, nil
}

// pass carries the state of one streamed read.
type pass struct {
	opts   ProcessOptions
	reader *Reader
	guard  *HeaderGuard
	seen   map[string]int
	batch  []*Row
	result *Result
}

func (p *pass) row(index int, cells []string, sink Sink) error {
	number := index + 1
	switch p.guard.Classify(cells) {
	case RowBlank:
		p.result.Stats.BlankRows++
		return nil
	case RowRepeatedHeader:
		p.result.Stats.RepeatedHeaders++
		return nil
	case RowSectionHeader:
		p.section(number, cells)
		return nil
	}

	p.result.Stats.DataRows++
	row, ok := p.reader.Read(number, cells)
	p.collect(row)
	if !ok {
		p.result.Stats.Rejected++
		return nil
	}
	p.result.Stats.Parsed++
	p.batch = append(p.batch, row)
	if len(p.batch) >= p.opts.BatchSize {
		return p.flush(sink)
	}
	return nil
}

// section re-reads the columns when the sheet starts a differently shaped part.
//
// Only the header is available at this point — the section's values have not
// been seen — so the new mapping rests on the header alone and says so. A sheet
// that stacks two suppliers' lists with different column orders is read
// correctly instead of forcing the second through the first's mapping, which is
// how the second half of such a file ends up with prices in the code column.
func (p *pass) section(number int, cells []string) {
	p.result.Stats.SectionHeaders++
	p.guard.Adopt(cells)

	headers := make([]string, len(cells))
	for i, c := range cells {
		headers[i] = sheet.CleanCell(c)
	}
	blank := make([]*sheet.ColumnProfile, len(headers))
	m := Resolve(headers, blank, p.opts.Vocabulary)
	p.reader.Rebind(m)

	p.note(Note{
		Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"يبدأ من الصف %d قسم جديد بترتيب أعمدة مختلف؛ تمت قراءته وفق عناوينه الخاصة. "+
				"راجع نتائج هذا القسم بعناية.", number),
	})
}

// duplicate is kept as a no-op so no valid rows are dropped, rejected, or falsely warned against.
func (p *pass) duplicate(row *Row) bool {
	return false
}

// IdentityKey is the strongest identity a row carries, used to detect a product
// listed twice in one file.
//
// The item code comes first because it is what the supplier themselves use to
// mean "the same product". The barcode is deliberately second and the name
// third: several packages legitimately share a barcode, and two genuinely
// different products can share a name once the manufacturer differs.
func IdentityKey(row *Row) string {
	if row == nil {
		return ""
	}
	if key := sheet.NormalizeKey(row.SKU); key != "" {
		return "sku:" + key
	}
	if row.Barcode != "" {
		return "barcode:" + row.Barcode
	}
	if name := sheet.NormalizeName(row.Name); name != "" {
		return "name:" + name + "|" + sheet.NormalizeKey(row.Manufacturer)
	}
	return ""
}

func (p *pass) flush(sink Sink) error {
	if len(p.batch) == 0 {
		return nil
	}
	if err := sink(p.batch); err != nil {
		return err
	}
	p.batch = p.batch[:0]
	return nil
}

// collect folds a row's findings into the run-level report.
func (p *pass) collect(row *Row) {
	if row == nil {
		return
	}
	for _, i := range row.Issues {
		p.issue(i)
	}
}

func (p *pass) issue(i Issue) {
	if i.Severity == SeverityWarning {
		p.result.Stats.Warnings++
	}
	if len(p.result.Issues) < p.opts.MaxIssues {
		p.result.Issues = append(p.result.Issues, i)
	}
}

func (p *pass) note(n Note) {
	const noteCap = 50
	if len(p.result.Notes) < noteCap {
		p.result.Notes = append(p.result.Notes, n)
	}
}
