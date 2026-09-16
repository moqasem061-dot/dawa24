package pages

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ImportReviewView is the review and confirmation screen.
type ImportReviewView struct {
	Session *catalog.ImportSession
	Counts  catalog.StagingCounts
	Rows    []*catalog.StagingRow
	Total   int

	// Structure is how the file was read, as stored on the session.
	Structure catalog.FileStructure
	Bindings  []ImportBindingRow
	Unmapped  []string

	Modes       []catalog.ImportModeOption
	Toggles     []ImportToggle
	Categories  []catalog.TaxonomyOption
	AIAvailable bool

	// Filter is the review table's current state, for the toolbar.
	Filter catalog.StagingFilter
	Page   int
	Pages  int

	Notice      string
	NoticeKind  string
	FatalDetail string

	// Progress is the live state of a background preparation run. While one is
	// in flight the review tables are meaningless — they still hold the previous
	// run's rows — so the page shows the progress panel instead.
	Progress catalog.ImportProgress
	Working  bool
}

// ProgressPercent is how far the run has got, or -1 when the phase carries no
// count and the bar should read as indeterminate.
func (v ImportReviewView) ProgressPercent() int { return v.Progress.Percent() }

// ProgressPhases lists the stages with the current one marked, so the admin can
// see what is happening rather than watching an unlabelled bar.
func (v ImportReviewView) ProgressPhases() []ImportPhaseInfo {
	phases := []catalog.ImportPhase{
		catalog.ImportPhaseReading,
		catalog.ImportPhaseParsing,
		catalog.ImportPhaseMapping,
		catalog.ImportPhaseMatching,
		catalog.ImportPhaseStaging,
	}

	current := -1
	for i, phase := range phases {
		if phase == v.Progress.Phase {
			current = i
		}
	}

	out := make([]ImportPhaseInfo, 0, len(phases))
	for i, phase := range phases {
		out = append(out, ImportPhaseInfo{
			Label:  phase.Label(),
			Active: i == current,
			Done:   current > i || v.Progress.Phase == catalog.ImportPhaseDone,
		})
	}
	return out
}

// ImportPhaseInfo is one stage on the progress panel.
type ImportPhaseInfo struct {
	Label  string
	Active bool
	Done   bool
}

// NewImportReviewView assembles the review screen.
func NewImportReviewView(
	session *catalog.ImportSession, counts catalog.StagingCounts,
	rows []*catalog.StagingRow, total int, filter catalog.StagingFilter,
	categories []catalog.TaxonomyOption, aiAvailable bool,
) ImportReviewView {
	view := ImportReviewView{
		Session:     session,
		Counts:      counts,
		Rows:        rows,
		Total:       total,
		Modes:       catalog.ImportModeOptions,
		Toggles:     importToggles(session.Options, aiAvailable),
		Categories:  categories,
		AIAvailable: aiAvailable,
		Filter:      filter,
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	view.Page = filter.Offset/limit + 1
	view.Pages = max((total+limit-1)/limit, 1)
	return view
}

// SetStructure fills the review screen's read-only account of how the file was
// interpreted, from the description stored on the session.
func (v *ImportReviewView) SetStructure(structure catalog.FileStructure) {
	v.Structure = structure
	for _, col := range structure.Columns {
		if col.Field == "" {
			if col.Header != "" {
				v.Unmapped = append(v.Unmapped, col.Header)
			}
			continue
		}
		v.Bindings = append(v.Bindings, ImportBindingRow{
			Column:     col.Letter,
			Header:     col.Header,
			Field:      catalog.FieldLabels[col.Field],
			Confidence: col.Confidence,
			Sample:     col.SampleText(),
		})
	}
}

// CanCommit reports whether there is anything selected to write.
func (v ImportReviewView) CanCommit() bool {
	return v.Session != nil && v.Session.IsReviewable() && v.Counts.Affected() > 0
}

// ModeIsDestructive reports whether the chosen strategy archives the catalogue,
// which the confirm button warns about.
func (v ImportReviewView) ModeIsDestructive() bool {
	return v.Session != nil && v.Session.Mode.IsDestructive()
}

// SourceSummary describes what was read, in one line.
func (v ImportReviewView) SourceSummary() string {
	if v.Session == nil {
		return ""
	}
	parts := []string{v.Session.Filename}
	switch v.Session.SourceFormat {
	case "xlsx":
		if v.Session.SheetName != "" {
			parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_32"), v.Session.SheetName))
		}
	case "csv":
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_33"), delimiterLabel(v.Session.Delimiter)))
	}
	if v.Session.BlockCount > 1 {
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.d_186"), v.Session.BlockCount))
	}
	parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.d_187"), v.Session.TotalRows))
	return strings.Join(parts, " · ")
}

// EnrichmentSummary is what the taxonomy passes resolved on this run.
func (v ImportReviewView) EnrichmentSummary() string {
	if v.Session == nil || v.Session.AINote == "" {
		return ""
	}
	if v.Session.AICalls == 0 {
		return v.Session.AINote
	}
	return fmt.Sprintf(i18n.TDefault("w4_ui.s_d_43"), v.Session.AINote, v.Session.AICalls)
}

// UsedAI reports whether a model was actually asked anything.
func (v ImportReviewView) UsedAI() bool { return v.Session != nil && v.Session.AICalls > 0 }

// MatchRate is the share of the file that resolved to a product the catalogue already holds.
func (v ImportReviewView) MatchRate() string {
	if v.Session == nil {
		return "—"
	}
	considered := v.Counts.Insert + v.Counts.Update
	if considered == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", v.Counts.Update*100/considered)
}

// RowActionBadge picks the badge colour for a staged action.
func RowActionBadge(action catalog.RowAction) string {
	switch action {
	case catalog.ActionInsert:
		return "badge-emerald"
	case catalog.ActionUpdate:
		return "badge-sky"
	default:
		return "badge-slate"
	}
}

// SessionStatusLabel renders a session state in the admin's language.
func SessionStatusLabel(status catalog.SessionStatus) string {
	switch status {
	case catalog.SessionDraft:
		return i18n.TDefault("w4_ui.s_188_188")
	case catalog.SessionProcessing:
		return i18n.TDefault("w4_ui.s_189_189")
	case catalog.SessionReady:
		return i18n.TDefault("w4_ui.s_190_190")
	case catalog.SessionCommitted:
		return i18n.TDefault("w4_ui.s_191_191")
	case catalog.SessionCancelled:
		return i18n.TDefault("w4_ui.s_192_192")
	case catalog.SessionFailed:
		return i18n.TDefault("w4_ui.s_193_193")
	default:
		return string(status)
	}
}

// SessionStatusBadge picks the badge colour for a session state.
func SessionStatusBadge(status catalog.SessionStatus) string {
	switch status {
	case catalog.SessionCommitted:
		return "badge-emerald"
	case catalog.SessionProcessing:
		return "badge-sky"
	case catalog.SessionReady:
		return "badge-amber"
	case catalog.SessionFailed:
		return "badge-rose"
	default:
		return "badge-slate"
	}
}

// ProductSummaryLine renders a staged product's details for the review table.
func ProductSummaryLine(row *catalog.StagingRow) string {
	if row == nil {
		return ""
	}
	return catalog.SummarizeProduct(row.Product)
}

// FormatCount renders a number with thousands separators.
func FormatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}

// importAction builds a POST target on the session.
func importAction(view ImportReviewView, verb string) templ.SafeURL {
	if verb == "" {
		return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s", view.Session.PublicID))
	}
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/%s", view.Session.PublicID, verb))
}

// importRowAction builds the include/exclude target for one staged row.
func importRowAction(view ImportReviewView, row *catalog.StagingRow) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/rows/%d",
		view.Session.PublicID, row.ID))
}

// includedToggleValue is what the row's button submits.
func includedToggleValue(row *catalog.StagingRow) string {
	if row.Included {
		return "0"
	}
	return "1"
}

// importFilterURL switches the active filter and returns to the first page.
func importFilterURL(view ImportReviewView, key string) templ.SafeURL {
	return templ.SafeURL("/admin/products/import/" + view.Session.PublicID +
		"?" + importQuery(view, key, 1).Encode())
}

func importQuery(view ImportReviewView, filterKey string, page int) url.Values {
	q := url.Values{}
	if filterKey != "" {
		q.Set("filter", filterKey)
	}
	if view.Filter.Search != "" {
		q.Set("q", view.Filter.Search)
	}
	if view.Filter.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", view.Filter.Limit))
	}
	if page > 1 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	return q
}

// currentFilterKey renders the active filter back into its query value.
func (v ImportReviewView) currentFilterKey() string {
	switch {
	case v.Filter.OnlyIssues:
		return "issues"
	case v.Filter.OnlyAI:
		return "ai"
	case v.Filter.Action != "":
		return string(v.Filter.Action)
	default:
		return ""
	}
}

// FilterIsActive reports whether a chip is the one currently applied.
func (v ImportReviewView) FilterIsActive(key string) bool {
	return v.currentFilterKey() == key
}

// ParseStagingFilter reads the review table's controls out of a query string.
func ParseStagingFilter(values url.Values, pageSize int) catalog.StagingFilter {
	filter := catalog.StagingFilter{
		Search: strings.TrimSpace(values.Get("q")),
		Limit:  pageSize,
	}

	switch values.Get("filter") {
	case string(catalog.ActionInsert), "new", "جديد", "الجديدة":
		filter.Action = catalog.ActionInsert
	case string(catalog.ActionUpdate), "تحديث", "التحديثات":
		filter.Action = catalog.ActionUpdate
	case string(catalog.ActionSkip), "تخطي":
		filter.Action = catalog.ActionSkip
	case "issues", "الملاحظات":
		filter.OnlyIssues = true
	case "ai", "استكمال ذكي":
		filter.OnlyAI = true
	}

	if page, err := strconv.Atoi(values.Get("page")); err == nil && page > 1 {
		filter.Offset = (page - 1) * pageSize
	}
	return filter
}

// SetRows fills the review table after the rows have been read.
func (v *ImportReviewView) SetRows(rows []*catalog.StagingRow, total int, counts catalog.StagingCounts) {
	v.Rows, v.Total, v.Counts = rows, total, counts

	limit := v.Filter.Limit
	if limit <= 0 {
		limit = 100
	}
	v.Page = v.Filter.Offset/limit + 1
	v.Pages = max((total+limit-1)/limit, 1)
}
