package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// IssueSeverity classifies what an issue means for the row it belongs to.
type IssueSeverity string

const (
	// SeverityError means the row was not imported.
	SeverityError IssueSeverity = "error"
	// SeverityWarning means the row was imported with an assumption applied.
	SeverityWarning IssueSeverity = "warning"
)

// RowIssue is one problem found in one row, addressed the way the admin sees
// their file: by spreadsheet row number and column heading.
type RowIssue struct {
	Row      int           `json:"row"`
	Column   string        `json:"column,omitempty"`
	Value    string        `json:"value,omitempty"`
	Message  string        `json:"message"`
	Severity IssueSeverity `json:"severity"`
}

// ImportStats counts what the parse and the write did.
type ImportStats struct {
	TotalRowsRead  int
	ValidProducts  int
	RepeatedHeader int
	EmptyRows      int
	DuplicateRows  int
	RejectedRows   int
	Warnings       int
	Inserted       int
	Updated        int
	HeaderRow      int
	HeaderBlocks   int
	SheetName      string
	Format         string
	Delimiter      string
}

// ParseResult is everything learned from reading one uploaded file.
type ParseResult struct {
	Products []*Product
	Stats    ImportStats
	Issues   []RowIssue
	Plan     ColumnPlan
	// MissingFields lists important fields no column supplied, as Arabic labels.
	MissingFields []string
	// SheetsSkipped names other worksheets that held data and were not read.
	SheetsSkipped []string
	// Layout is the block structure the sheet was read as.
	Layout SheetLayout
	// options are the enrichment switches this parse ran under.
	options ImportOptions
	// SourceRows holds the spreadsheet row number each product came from,
	// parallel to Products. The staging table records it so a finding raised
	// after the write still points at a row in the admin's own file.
	SourceRows []int
}

// HasErrors reports whether any row was rejected.
func (r *ParseResult) HasErrors() bool { return r.Stats.RejectedRows > 0 }

// maxStorablePriceMinor is the largest value catalog.products' NUMERIC(12,2)
// price columns can hold, in piastres: 9,999,999,999.99.
const maxStorablePriceMinor int64 = 999999999999

// maxIssues caps what is retained. A file with 9,000 broken rows must not turn
// one bad upload into a gigabyte of report, and no admin reads past the first
// few hundred; the counters stay exact regardless.
const maxIssues = 500

// dosageFormKeywords maps words that appear in an Egyptian product name onto the
// pharmaceutical form. Longer, more specific entries come first: "غسول فم" must
// win over "غسول", and "معجون أسنان" over any bare match.
var dosageFormKeywords = []struct {
	keywords []string
	form     string
}{
	{[]string{"غسول فم", "مضمضة", "مضمضه", "mouthwash"}, "غسول فم"},
	{[]string{"معجون اسنان", "معجون أسنان", "toothpaste"}, "معجون أسنان"},
	{[]string{"غسول", "wash", "lotion", "لوشن"}, "غسول"},
	{[]string{"كريم", "cream"}, "كريم"},
	{[]string{"مرهم", "ointment", "oint"}, "مرهم"},
	{[]string{"جل", "جيل", "gel"}, "جل"},
	{[]string{"زيت", "oil", "اويل"}, "زيت"},
	{[]string{"سيروم", "serum"}, "سيروم"},
	{[]string{"شامبو", "shampoo"}, "شامبو"},
	{[]string{"صابون", "صابونة", "صابونه", "soap"}, "صابون"},
	{[]string{"رول اون", "roll on", "رول-اون", "رول_اون"}, "رول اون"},
	{[]string{"اسبراي", "سبراي", "اسبراى", "سبراى", "بخاخ", "spray", "بدى ميست", "body mist"}, "بخاخ / اسبراي"},
	{[]string{"مناديل", "wipes"}, "مناديل مبللة"},
	{[]string{"صبغة", "صبغه", "hair color", "color", "colour"}, "صبغة شعر"},
	{[]string{"اقراص", "أقراص", "قرص", "tab", "tabs", "tablet", "tablets"}, "أقراص"},
	{[]string{"كبسول", "كبسولات", "cap", "caps", "capsule", "capsules"}, "كبسولات"},
	{[]string{"شراب", "syrup", "susp", "معلق"}, "شراب"},
	{[]string{"نقط", "drops", "قطرة", "قطره"}, "نقط"},
	{[]string{"حقن", "حقنة", "حقنه", "امبول", "أمبول", "امبولات", "أمبولات", "فيال", "vial", "ampoule", "inj"}, "حقن وأمبولات"},
	{[]string{"فوار", "ساشيت", "sachet", "eff"}, "أكياس فوار"},
	{[]string{"لبوس", "تحاميل", "تحميلة", "supp", "suppository"}, "لبوس"},
	{[]string{"حفاضات", "حفاضه", "diapers"}, "مستلزمات عناية"},
	{[]string{"استيك", "ستيك", "stick"}, "ستيك مضاد للتعرق"},
}

// defaultDosageForm labels a product whose name gives no clue about its form.
const defaultDosageForm = "مستحضر صيدلاني"

var (
	concentrationPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s*(?:ملجرام|مليجرام|مجم|جم|جرام|مل|ملي|ملم|mg|g|gm|ml|mcg|iu|%|spf[+\d]*|[+\d]*spf))`)
	digitsOnlyPattern    = regexp.MustCompile(`^[\d\s./\-]+$`)
)

// ExtractDosageAndConcentration infers the pharmaceutical form and strength from
// a product name, for the very common supplier file that carries neither as its
// own column.
func ExtractDosageAndConcentration(name string) (dosage string, conc string) {
	lowered := strings.ToLower(name)
	for _, dk := range dosageFormKeywords {
		for _, kw := range dk.keywords {
			if strings.Contains(lowered, kw) {
				dosage = dk.form
				break
			}
		}
		if dosage != "" {
			break
		}
	}
	if dosage == "" {
		dosage = defaultDosageForm
	}

	if match := concentrationPattern.FindString(NormalizeDigits(name)); match != "" {
		conc = CleanCellString(match)
	}
	return dosage, conc
}

// DetectHeaderRow scans the head of the file for the row that names the columns.
//
// Scanning only the first rows is deliberate: a label row appearing at row 400
// is a repeated print header, which is handled separately.
func DetectHeaderRow(records [][]string) int {
	bestIdx, bestScore := -1, 0

	for i := 0; i < min(40, len(records)); i++ {
		// Later rows that score equally are usually the first data row echoing
		// the header's vocabulary, so earlier candidates win ties.
		score := scoreHeaderCandidate(records[i]) - i
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	return bestIdx
}

// scoreHeaderCandidate rates how much one row looks like a set of column titles.
//
// It rewards cells that identify a known field, and penalises the two things
// that mark a row as data rather than titles: mostly numeric cells, and repeated
// values. A header row names things; a data row states them.
func scoreHeaderCandidate(row []string) int {
	if len(row) == 0 {
		return 0
	}
	plan := PlanColumns(row, nil)
	if len(plan.Bindings) == 0 {
		return 0
	}

	filled, numeric, duplicates := 0, 0, 0
	seen := map[string]bool{}
	for _, cell := range row {
		clean := CleanCellString(cell)
		if clean == "" {
			continue
		}
		filled++
		if digitsOnlyPattern.MatchString(NormalizeDigits(clean)) {
			numeric++
		}
		key := NormalizeKey(clean)
		if key != "" && seen[key] {
			duplicates++
		}
		seen[key] = true
	}
	if filled == 0 {
		return 0
	}

	score := 0
	for _, b := range plan.Bindings {
		score += b.Score
	}
	return score - numeric*40 - duplicates*30
}

// ParseProducts converts a decoded sheet into products, with a full account of
// what was skipped, assumed, and rejected.
func ParseProducts(data *SheetData) *ParseResult {
	return ParseProductsWithOverrides(data, LayoutOverrides{})
}

// ParseProductsWithOverrides reads a sheet after applying the admin's
// corrections to the detected structure, inferring everything it can.
func ParseProductsWithOverrides(data *SheetData, overrides LayoutOverrides) *ParseResult {
	return ParseSheet(data, overrides, ImportOptions{AssignDosageForm: true})
}

// ParseSheet reads a sheet under the admin's structural corrections and their
// enrichment switches.
//
// The switches reach this far down because some of them change how a row is
// read, not just what happens to it afterwards. Inferring the pharmaceutical
// form from a product's name used to happen unconditionally, which meant
// turning "تحديد الشكل الصيدلي" off still wrote a form — and a switch that does
// nothing is worse than no switch, because it is believed.
func ParseSheet(data *SheetData, overrides LayoutOverrides, opts ImportOptions) *ParseResult {
	res := &ParseResult{Stats: ImportStats{HeaderRow: -1}, options: opts}
	if data == nil || len(data.Rows) == 0 {
		return res
	}

	res.Stats.TotalRowsRead = len(data.Rows)
	res.Stats.SheetName = data.Sheet
	res.Stats.Format = data.Format
	res.Stats.Delimiter = data.Delimiter
	res.SheetsSkipped = data.SheetsSkipped

	res.Layout = AnalyzeLayout(data).Apply(data, overrides)
	res.Plan = res.Layout.Primary
	if len(res.Layout.HeaderRows) > 0 {
		res.Stats.HeaderRow = res.Layout.HeaderRows[0]
	}
	res.Stats.HeaderBlocks = len(res.Layout.Blocks)
	// Every header after the first is a reprint from a paginated export. They
	// are block boundaries rather than rows inside a block, so nothing skips
	// them row by row — but they must still be counted, or the admin's row
	// arithmetic silently fails to add up to the file's length.
	if n := len(res.Layout.HeaderRows); n > 1 {
		res.Stats.RepeatedHeader = n - 1
	}
	res.reportStructure()

	res.Products = res.collectProducts(data.Rows)
	res.Stats.ValidProducts = len(res.Products)
	sort.SliceStable(res.Issues, func(i, j int) bool { return res.Issues[i].Row < res.Issues[j].Row })
	return res
}

// reportStructure tells the admin what shape the file was read as, before any
// row is interpreted. A wrong reading here mislabels the entire catalogue, so it
// is surfaced rather than left implicit.
func (r *ParseResult) reportStructure() {
	if r.Layout.Positional {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: "تعذر التعرف على صف العناوين في الملف، وتمت قراءة الأعمدة بالترتيب " +
				"(كود الصنف، اسم الصنف، الشركة المصنعة). يمكنك تصحيح ذلك من خطوة مراجعة الأعمدة.",
		})
	}
	if n := len(r.Layout.HeaderRows); n > 1 {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"الملف مقسّم إلى %d كتلة بيانات يتكرر فيها صف العناوين (تصدير مُقسّم للطباعة). "+
					"تمت قراءة جميع الكتل، وتم تجاهل صفوف العناوين المتكررة.", n),
		})
	}
	if r.Layout.VariantBlocks > 0 {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"يحتوي الملف على %d قسم بترتيب أعمدة مختلف عن القسم الأول؛ تمت قراءة كل قسم وفق عناوينه الخاصة.",
				r.Layout.VariantBlocks),
		})
	}

	r.MissingFields = missingImportantFields(r.Plan)
	for _, label := range r.MissingFields {
		r.addIssue(RowIssue{
			Row:      1,
			Column:   label,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"لم يتم العثور على عمود «%s» في الملف. سيتم استيراد الأصناف بدون هذه البيانات.", label),
		})
	}
}
