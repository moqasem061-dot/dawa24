package productmatch

import (
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Confidence buckets a binding for the review screen.
type Confidence string

const (
	// ConfidenceCertain means both witnesses agree, or one is conclusive.
	ConfidenceCertain Confidence = "certain"
	// ConfidenceHigh means the binding is sound but worth a glance.
	ConfidenceHigh Confidence = "high"
	// ConfidenceMedium means the vendor should confirm it.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceLow means the engine is guessing and says so.
	ConfidenceLow Confidence = "low"
)

// Label renders a confidence level in Arabic.
func (c Confidence) Label() string {
	switch c {
	case ConfidenceCertain:
		return "مؤكد"
	case ConfidenceHigh:
		return "مرجّح"
	case ConfidenceMedium:
		return "يحتاج مراجعة"
	default:
		return "غير مؤكد"
	}
}

// NeedsReview reports whether a binding should be flagged to the vendor.
func (c Confidence) NeedsReview() bool {
	return c == ConfidenceMedium || c == ConfidenceLow
}

// Source records who decided a binding.
type Source string

const (
	// SourceAuto is the analyser's own decision.
	SourceAuto Source = "auto"
	// SourceManual is the vendor's override.
	SourceManual Source = "manual"
	// SourceCompleted is a binding the engine filled in after the vendor's
	// review, for a field they left unmapped.
	SourceCompleted Source = "completed"
)

// Candidate is one field a column could be, with the evidence for it.
type Candidate struct {
	Field Field    `json:"field"`
	Label string   `json:"label"`
	Score float64  `json:"score"`
	Why   []string `json:"why,omitempty"`
}

// Column is one spreadsheet column and what became of it.
type Column struct {
	Index   int    `json:"index"`
	Header  string `json:"header"`
	Field   Field  `json:"field,omitempty"`
	Ignored bool   `json:"ignored"`

	Score      float64    `json:"score"`
	Confidence Confidence `json:"confidence,omitempty"`
	Source     Source     `json:"source,omitempty"`
	// Why explains the binding in Arabic, one clause per witness.
	Why []string `json:"why,omitempty"`
	// Candidates are the other fields this column scored for, best first, which
	// is what the review screen offers in its dropdown.
	Candidates []Candidate `json:"candidates,omitempty"`
	// Preview holds a few real values, so the vendor recognises the column
	// without opening their own file beside the screen.
	Preview []string `json:"preview,omitempty"`
	// Profile is the measured evidence, kept for the explanation panel.
	Profile *sheet.ColumnProfile `json:"profile,omitempty"`
}

// Conflict is something the vendor must look at before the import runs.
type Conflict struct {
	Kind     ConflictKind `json:"kind"`
	Field    Field        `json:"field,omitempty"`
	Column   int          `json:"column"`
	Severity Severity     `json:"severity"`
	Message  string       `json:"message"`
}

// ConflictKind classifies why a binding was flagged.
type ConflictKind string

const (
	// ConflictHeaderValue means the header names one field and the values look
	// like another. This is the finding that saves imports.
	ConflictHeaderValue ConflictKind = "header_value"
	// ConflictAmbiguous means two readings scored too close to separate.
	ConflictAmbiguous ConflictKind = "ambiguous"
	// ConflictDuplicateHeader means two columns carry the same title.
	ConflictDuplicateHeader ConflictKind = "duplicate_header"
	// ConflictMissing means a field the import depends on found no column.
	ConflictMissing ConflictKind = "missing"
	// ConflictInconsistent comes from the cross-column arithmetic checks.
	ConflictInconsistent ConflictKind = "inconsistent"
)

// Mapping is the resolved reading of a sheet's columns.
type Mapping struct {
	Columns   []*Column     `json:"columns"`
	ByField   map[Field]int `json:"by_field"`
	Conflicts []Conflict    `json:"conflicts,omitempty"`
	Notes     []Note        `json:"notes,omitempty"`
}

// Column returns the column bound to a field, and whether one is.
func (m *Mapping) Column(f Field) (int, bool) {
	idx, ok := m.ByField[f]
	return idx, ok
}

// Has reports whether a field is bound.
func (m *Mapping) Has(f Field) bool {
	_, ok := m.ByField[f]
	return ok
}

// Bound lists the bound fields in catalogue order, which is the order the
// review screen renders.
func (m *Mapping) Bound() []Field {
	var out []Field
	for _, spec := range Specs {
		if m.Has(spec.Field) {
			out = append(out, spec.Field)
		}
	}
	return out
}

// Unmapped lists the columns no field claimed.
func (m *Mapping) Unmapped() []*Column {
	var out []*Column
	for _, c := range m.Columns {
		if c.Field == "" && !c.Ignored {
			out = append(out, c)
		}
	}
	return out
}

// NeedsReview reports whether anything is flagged for the vendor's attention.
func (m *Mapping) NeedsReview() bool {
	for _, c := range m.Conflicts {
		if c.Severity != SeverityInfo {
			return true
		}
	}
	for _, c := range m.Columns {
		if c.Field != "" && c.Confidence.NeedsReview() {
			return true
		}
	}
	return false
}

// pair is one scored (field, column) reading awaiting assignment.
type pair struct {
	field  Field
	column int
	score  float64
	why    []string
	// headerOnly marks a reading the values could not corroborate, which the
	// conflict pass reports.
	headerScore float64
	valueScore  float64
	hasValue    bool
	// namedExactly marks a reading whose column header *is* this field's name,
	// as opposed to merely containing a word associated with it.
	namedExactly bool
}

// minPairScore is the floor below which a reading is not offered at all. It is
// low on purpose: a weak reading still belongs in the review screen's dropdown,
// where the vendor can pick it, even though it is too weak to apply by itself.
const minPairScore = 0.22

// minBindScore is the floor for binding a field without being asked.
const minBindScore = 0.34

// uncorroboratedCap is the ceiling on a reading that neither the header
// supports nor the values settle. It sits below minBindScore on purpose: such a
// reading is offered in the review dropdown and never applied by itself.
const uncorroboratedCap = 0.30

// score combines the two witnesses.
//
// The combination is a maximum of three readings rather than a single weighted
// sum, because a sum dilutes conclusive evidence. A column of valid GS1 check
// digits under a header nobody wrote is certainly a barcode; a weighted average
// against a zero header would score it 0.45 and leave it unmapped, which is a
// worse answer than either witness gave on its own.
func combineScores(headerScore int, v verdict, hasDetector bool) (float64, []string) {
	// A field whose values do not identify it on their own needs the header to
	// vouch for it. Half the catalogue is in that position — a small whole
	// number is equally a pack size, a minimum order and a re-order level — and
	// binding those on shape alone is how an import invents data the file never
	// contained. Capping below the binding threshold keeps them available in
	// the review dropdown while refusing to assert them.
	header := clamp(float64(headerScore) / 130)
	var why []string
	if headerScore >= scoreExact {
		why = append(why, "عنوان العمود مطابق تماماً لهذا الحقل")
	} else if headerScore >= scoreStrong {
		why = append(why, "عنوان العمود يشير بوضوح إلى هذا الحقل")
	} else if headerScore >= scoreFloor {
		why = append(why, "عنوان العمود يحتمل هذا الحقل")
	}
	if v.why != "" {
		why = append(why, v.why)
	}

	// An exact header match is the file stating what the column is, in the
	// field's own name, with nothing else plausible about it. The values may
	// still veto it — that happens before this function is reached — but short
	// of a veto there is nothing left to be uncertain about, and reporting such
	// a binding as a guess is what made a perfectly named column ask for AI
	// help and show "تخميني" on the review screen.
	//
	// Corroborating values raise it further; their absence does not lower it,
	// because a header-only file has no values to disagree with.
	if headerScore >= scoreExact {
		return clamp(max(0.85, 0.85+0.15*v.score)), why
	}

	if !hasDetector {
		// Nothing measurable distinguishes this field's values, so the header
		// carries it alone — and is capped, because an uncorroborated header is
		// never certain.
		return clamp(header * 0.8), why
	}
	if headerScore < scoreFloor && !v.decisive {
		return clamp(min(v.score*0.85, uncorroboratedCap)), why
	}

	blended := 0.55*header + 0.45*v.score
	if header >= 0.5 && v.score >= 0.55 {
		blended += 0.15 // both witnesses agree
	}
	best := blended
	if solo := v.score * 0.85; solo > best {
		best = solo
	}
	if solo := header * 0.8; solo > best {
		best = solo
	}
	return clamp(best), why
}

// confidenceOf buckets a score.
func confidenceOf(score float64) Confidence {
	switch {
	case score >= 0.82:
		return ConfidenceCertain
	case score >= 0.62:
		return ConfidenceHigh
	case score >= 0.42:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

// Resolve reads a sheet's headers and profiled values into a mapping, offering
// every field in the catalogue.
func Resolve(headers []string, profiles []*sheet.ColumnProfile, vocab *Vocabulary) *Mapping {
	return ResolveWith(headers, profiles, vocab, nil)
}

// ResolveWith reads a sheet under an explicit field set. A nil set allows every
// field, which is what Resolve passes.
func ResolveWith(headers []string, profiles []*sheet.ColumnProfile, vocab *Vocabulary, fields *FieldSet) *Mapping {
	if vocab == nil {
		vocab = &Vocabulary{}
	}
	m := &Mapping{ByField: map[Field]int{}}

	shapes := make([]*shape, len(profiles))
	for i, p := range profiles {
		shapes[i] = newShape(p)
		header := ""
		if i < len(headers) {
			header = headers[i]
		}
		m.Columns = append(m.Columns, &Column{
			Index:   i,
			Header:  header,
			Profile: p,
			Preview: previewOf(p),
		})
	}

	pairs, vetoes := scorePairs(headers, shapes, vocab, fields)
	assign(m, pairs)
	// The one place value evidence may overturn a settled header binding. See
	// identity_swap.go for why it is limited to this pair.
	fixIdentitySwap(m, shapes, headers)
	attachCandidates(m, pairs)
	flagConflicts(m, pairs, vetoes, headers)
	return m
}
