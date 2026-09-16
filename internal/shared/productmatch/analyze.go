package productmatch

import (
	"fmt"
	"math"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The analysis stage.
//
// One bounded pass over the head of the file answers every question the review
// screen asks: what shape is this sheet, what is each column, how sure are we,
// and what should the vendor look at before anything is written.
//
// Nothing is persisted from here except the vendor's own choices. The analysis
// is deterministic, so the processing stage re-derives it from the same file
// and gets the same answer rather than trusting a serialised snapshot — which
// also means a mapping cannot drift between what the vendor confirmed and what
// the import then used.

// profileRows is how many data rows the profiler reads. Two thousand is far
// past the point where another row changes a percentile, and it costs a few
// milliseconds even on a workbook of a hundred thousand.
const profileRows = 2000

// previewRows is how many rows the review screen shows.
const previewRows = 15

// Analysis is everything learned about an uploaded file.
type Analysis struct {
	Source sheet.Source `json:"source"`
	Layout Layout       `json:"layout"`
	// Mapping is the resolved reading of the columns, and the thing the vendor
	// reviews and corrects.
	Mapping *Mapping `json:"mapping"`
	// Preview is the head of the sheet as the vendor would see it, header row
	// included, for the table on the review screen.
	Preview [][]string `json:"preview"`
	// Sampled is how many data rows the profiler actually read.
	Sampled int `json:"sampled"`
	// Skipped counts what the sample passed over, so the arithmetic on screen
	// adds up to the file's length.
	BlankRows    int `json:"blank_rows"`
	RepeatedRows int `json:"repeated_rows"`
	// Relation is the arithmetic identity found between the price columns, if
	// any. Its presence is the strongest guarantee the pricing was read right.
	Relation *Relation `json:"relation,omitempty"`

	grid   NumericGrid
	fields *FieldSet
	shapes []*shape
	pairs  []pair
	vocab  *Vocabulary
}

// Analyze reads the head of an opened file and resolves its columns.
func Analyze(book *sheet.Book, vocab *Vocabulary) (*Analysis, error) {
	return AnalyzeWith(book, AnalyzeOptions{Vocabulary: vocab})
}

// AnalyzeOptions narrows what the analysis is allowed to conclude.
type AnalyzeOptions struct {
	// Vocabulary is the catalogue-derived value lists that make the mapping
	// schema-aware rather than merely rule-based.
	Vocabulary *Vocabulary
	// Fields is the subset of the catalogue this importer can use. A nil set
	// allows everything, which is what a vendor price list needs; a pharmacy's
	// shopping list needs four fields and should never be offered the other
	// twenty-five. See fieldset.go.
	Fields *FieldSet
}

// AnalyzeWith runs the analysis under an explicit field set.
func AnalyzeWith(book *sheet.Book, opts AnalyzeOptions) (a *Analysis, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر تحليل هيكل ملف البيانات (%v)", r)
			a = nil
		}
	}()

	if book == nil {
		return nil, fmt.Errorf("لم يتم فتح أي ملف للتحليل")
	}
	vocab := opts.Vocabulary
	if vocab == nil {
		vocab = &Vocabulary{}
	}
	fields := opts.Fields

	head, err := book.Peek(sheet.DefaultPeekRows)
	if err != nil {
		return nil, err
	}

	layout, notes := AnalyzeLayout(head.Rows)
	a = &Analysis{
		Source: book.Source(),
		Layout: layout,
		vocab:  vocab,
	}
	a.Preview = previewSlice(head.Rows, layout)

	profiles, err := a.profile(book, layout)
	if err != nil {
		return nil, err
	}

	a.Mapping = ResolveWith(layout.Headers, profiles, vocab, fields)
	a.Mapping.Notes = append(a.Mapping.Notes, notes...)
	a.shapes = make([]*shape, len(profiles))
	for i, p := range profiles {
		a.shapes[i] = newShape(p)
	}
	a.fields = fields
	a.pairs, _ = scorePairs(layout.Headers, a.shapes, vocab, fields)

	if rel, ok := FindPriceRelation(a.grid, a.shapes); ok {
		ApplyRelation(a.Mapping, rel)
		a.Relation = &rel
	}
	a.Revalidate()
	return a, nil
}

// profile streams the head of the sheet, measuring every column and building
// the row-aligned numeric grid the cross-column checks need.
func (a *Analysis) profile(book *sheet.Book, layout Layout) ([]*sheet.ColumnProfile, error) {
	width := layout.Width
	profiles := make([]*sheet.ColumnProfile, width)
	for i := 0; i < width; i++ {
		header := ""
		if i < len(layout.Headers) {
			header = layout.Headers[i]
		}
		profiles[i] = sheet.NewColumnProfile(i, header)
	}
	a.grid = NumericGrid{Width: width}

	guard := NewHeaderGuard(layout.Headers)
	err := book.Walk(func(index int, row []string) error {
		if index < layout.FirstDataRow {
			return nil
		}
		switch guard.Classify(row) {
		case RowBlank:
			a.BlankRows++
			return nil
		case RowRepeatedHeader:
			a.RepeatedRows++
			return nil
		case RowSectionHeader:
			// A differently shaped section starts here. Its columns are read on
			// their own titles during the import; for profiling, the section's
			// header row is not data and its values still belong to the same
			// physical columns, so the walk simply continues.
			a.RepeatedRows++
			guard.Adopt(row)
			return nil
		}

		numbers := make([]float64, width)
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			profiles[i].Observe(cell)
			numbers[i] = math.NaN()
			if d, err := sheet.Coerce(cell); err == nil {
				numbers[i] = d.Float
			}
		}
		a.grid.Rows = append(a.grid.Rows, numbers)

		a.Sampled++
		if a.Sampled >= profileRows {
			return sheet.ErrStop
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		p.Seal()
	}
	return profiles, nil
}

// previewSlice takes the header row and the first data rows for the screen.
func previewSlice(rows [][]string, layout Layout) [][]string {
	start := layout.HeaderRow
	if start < 0 {
		start = 0
	}
	end := min(start+previewRows+1, len(rows))
	out := make([][]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, padRow(rows[i], layout.Width))
	}
	return out
}

// IgnoreField marks a column the vendor deliberately excluded, so the
// completion pass does not helpfully bind it to something.
const IgnoreField Field = "-"

// ApplyOverrides folds the vendor's corrections into the mapping.
//
// Overrides are absolute. A field the vendor moved is moved, whatever the
// evidence said, and whatever previously held that field or that column is
// released. The review step exists so their judgement wins; an override that
// the engine then argued with would make the step theatre.
func (a *Analysis) ApplyOverrides(overrides map[int]Field) {
	if len(overrides) == 0 {
		return
	}
	m := a.Mapping
	for col, f := range overrides {
		if col < 0 || col >= len(m.Columns) {
			continue
		}
		c := m.Columns[col]
		// Release whatever this column held.
		if old := c.Field; old != "" {
			delete(m.ByField, old)
			c.Field = ""
		}
		c.Ignored = false
		c.Confidence = ""
		c.Score = 0

		switch f {
		case "":
			c.Why = []string{"تم إلغاء ربط هذا العمود يدوياً"}
			continue
		case IgnoreField:
			c.Ignored = true
			c.Why = []string{"تم استبعاد هذا العمود يدوياً من الاستيراد"}
			continue
		}
		if _, known := SpecOf(f); !known {
			continue
		}
		// Release whatever else held this field.
		if prev, ok := m.ByField[f]; ok && prev != col {
			m.Columns[prev].Field = ""
			m.Columns[prev].Confidence = ""
			m.Columns[prev].Why = append(m.Columns[prev].Why,
				fmt.Sprintf("تم نقل حقل «%s» إلى عمود آخر يدوياً", f.Label()))
		}
		m.ByField[f] = col
		c.Field = f
		c.Source = SourceManual
		c.Score = 1
		c.Confidence = ConfidenceCertain
		c.Why = []string{"تم تحديد هذا الحقل يدوياً بواسطة المستخدم"}
	}
	a.Revalidate()
}

// minCompleteScore is the floor for a binding the engine adds on the vendor's
// behalf after their review. It is below the automatic threshold because the
// vendor has now seen the mapping and left these columns alone, so a weaker
// reading is worth offering than would have been worth asserting up front.
const minCompleteScore = 0.28

// Complete binds the fields the vendor left unmapped to the columns they left
// unclaimed, wherever the evidence supports it.
//
// This is the fifth step of the review flow: the vendor corrects what matters
// to them, and the engine finishes the rest rather than dropping columns it
// understood perfectly well. Every binding made here is marked as such and
// listed in the confirmation, so nothing is added behind the vendor's back.
func (a *Analysis) Complete() []Note {
	m := a.Mapping
	taken := make(map[int]bool, len(m.Columns))
	for _, c := range m.Columns {
		if c.Field != "" || c.Ignored {
			taken[c.Index] = true
		}
	}

	var notes []Note
	for _, p := range a.pairs {
		if p.score < minCompleteScore {
			break // sorted best-first
		}
		if taken[p.column] {
			continue
		}
		if _, done := m.ByField[p.field]; done {
			continue
		}
		m.ByField[p.field] = p.column
		taken[p.column] = true

		c := m.Columns[p.column]
		c.Field = p.field
		c.Score = p.score
		c.Confidence = confidenceOf(p.score)
		c.Source = SourceCompleted
		c.Why = append(p.why, "تم ربط هذا العمود تلقائياً بعد المراجعة لأنه لم يُحدد يدوياً")
		notes = append(notes, Note{
			Severity: SeverityInfo,
			Message: fmt.Sprintf("تم ربط العمود «%s» بحقل «%s» تلقائياً (لم يكن محدداً في المراجعة).",
				headerOf(m, p.column), p.field.Label()),
		})
	}
	if len(notes) > 0 {
		m.Notes = append(m.Notes, notes...)
		a.Revalidate()
	}
	return notes
}

// Revalidate recomputes the conflicts that depend on the current bindings.
//
// The structural conflicts — a header disagreeing with its values, two readings
// scoring alike — are properties of the analysis and survive. The relational
// ones depend on what is bound right now and are rebuilt, so a vendor who
// swaps two columns immediately sees whether the swap fixed the inconsistency
// or created one.
func (a *Analysis) Revalidate() {
	kept := a.Mapping.Conflicts[:0]
	for _, c := range a.Mapping.Conflicts {
		if c.Kind != ConflictInconsistent && c.Kind != ConflictMissing {
			kept = append(kept, c)
		}
	}
	a.Mapping.Conflicts = kept
	a.Mapping.Conflicts = append(a.Mapping.Conflicts, CheckOrdering(a.Mapping, a.grid)...)
	a.Mapping.Conflicts = append(a.Mapping.Conflicts, CheckMissingWith(a.Mapping, a.fields)...)
}

// Blocking reports the findings that must be resolved before the import can
// run at all, as opposed to the ones the vendor may knowingly accept.
func (a *Analysis) Blocking() []Conflict {
	var out []Conflict
	for _, c := range a.Mapping.Conflicts {
		if c.Severity == SeverityError {
			out = append(out, c)
		}
	}
	return out
}
