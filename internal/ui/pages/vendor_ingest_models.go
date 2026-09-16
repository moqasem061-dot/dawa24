package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// VendorImportView is everything the import screen renders.
type VendorImportView struct {
	Lang     string
	Session  *ingest.Session
	Analysis *productmatch.Analysis

	Warehouses []*inventory.Warehouse
	Recent     []*ingest.Session

	// Rows and its companions back the results table.
	Rows      []*ingest.RowOutcome
	RowTotal  int
	RowCounts map[string]int
	Filter    ingest.RowFilter
	Page      int
	PerPage   int

	AIAvailable         bool
	AIUnavailableReason string

	// Plan is what committing this import would do under the chosen mode.
	Plan *ingest.CommitPlan

	NoticeType    string
	NoticeMessage string
	Fatal         string

	// Target organization context when executed by staff on behalf of an org
	Audience      string
	TargetOrgID   int64
	TargetOrgName string
	BaseURL       string
}

// MappedIdentifiers reports which identifier columns the vendor bound in step
// one, so the settings screen can offer a toggle only where it could do
// something.
//
// A switch for a column that was never mapped is worse than a missing switch: a
// vendor ticks it, nothing changes, and they conclude the matching is broken.
// The live analysis is preferred over the stored snapshot because a vendor who
// steps back and re-maps sees the effect immediately.
func (v VendorImportView) MappedIdentifiers() productmatch.MappedColumns {
	if v.Analysis != nil && v.Analysis.Mapping != nil {
		return v.Analysis.Mapping.MappedIdentifiers()
	}
	if v.Session != nil {
		return v.Session.Mapping.MappedIdentifiers()
	}
	return productmatch.MappedColumns{}
}

// Phase is the stage the screen should render.
func (v VendorImportView) Phase() ingest.Phase {
	if v.Session == nil {
		return ""
	}
	return v.Session.Phase
}

// Started reports whether an import is open at all, as opposed to the upload
// screen.
func (v VendorImportView) Started() bool { return v.Session != nil }

// WizardStep maps the session's phase onto the shared, canonically numbered
// step. The phase is the vendor import's own state machine and stays that way;
// only what the rail shows is shared.
func (v VendorImportView) WizardStep() Step {
	switch v.Phase() {
	case ingest.PhaseMapping:
		return StepColumns
	case ingest.PhaseSettings:
		return StepSettings
	case ingest.PhaseReview, ingest.PhaseConfirm, ingest.PhaseProcessing:
		return StepReview
	case ingest.PhaseCompleted, ingest.PhaseFailed, ingest.PhaseCancelled:
		return StepResults
	default:
		return StepFile
	}
}

// MappedColumns are the analysed columns, or the stored snapshot once the run
// has finished and the file may already have been reaped.
func (v VendorImportView) MappedColumns() []*productmatch.Column {
	if v.Analysis != nil && v.Analysis.Mapping != nil {
		return v.Analysis.Mapping.Columns
	}
	return nil
}

// CoreFieldStatus tracks the mapping state of critical and required catalog & inventory fields.
type CoreFieldStatus struct {
	Field       productmatch.Field
	Label       string
	Description string
	Required    bool
	ColumnIndex int
	ColumnName  string
}

// CoreFieldsStatus inspects all mapped columns and reports status for every essential field.
func (v VendorImportView) CoreFieldsStatus() []CoreFieldStatus {
	lang := v.Lang
	if lang == "" {
		lang = "ar"
	}

	cols := v.MappedColumns()
	colMap := make(map[productmatch.Field]*productmatch.Column)
	for _, c := range cols {
		if !c.Ignored {
			colMap[c.Field] = c
		}
	}

	colName := func(f productmatch.Field) (int, string) {
		if c, ok := colMap[f]; ok {
			if c.Header != "" {
				return c.Index, c.Header
			}
			return c.Index, fmt.Sprintf(i18n.T(lang, "ingest.field.column_n"), c.Index+1)
		}
		return -1, ""
	}

	idxName, nameName := colName(productmatch.FieldName)
	idxPrice, namePrice := colName(productmatch.FieldPrice)
	idxQty, nameQty := colName(productmatch.FieldQuantity)
	idxSKU, nameSKU := colName(productmatch.FieldSKU)
	idxBar, nameBar := colName(productmatch.FieldBarcode)
	idxExp, nameExp := colName(productmatch.FieldExpiryDate)
	idxDisc, nameDisc := colName(productmatch.FieldDiscountPct)

	return []CoreFieldStatus{
		{
			Field:       productmatch.FieldName,
			Label:       i18n.T(lang, "ingest.field.name"),
			Description: i18n.T(lang, "ingest.field.name_desc"),
			Required:    true,
			ColumnIndex: idxName,
			ColumnName:  nameName,
		},
		{
			Field:       productmatch.FieldPrice,
			Label:       i18n.T(lang, "ingest.field.price"),
			Description: i18n.T(lang, "ingest.field.price_desc"),
			Required:    true,
			ColumnIndex: idxPrice,
			ColumnName:  namePrice,
		},
		{
			Field:       productmatch.FieldQuantity,
			Label:       i18n.T(lang, "ingest.field.quantity"),
			Description: i18n.T(lang, "ingest.field.quantity_desc"),
			Required:    true,
			ColumnIndex: idxQty,
			ColumnName:  nameQty,
		},
		{
			Field:       productmatch.FieldSKU,
			Label:       i18n.T(lang, "ingest.field.sku"),
			Description: i18n.T(lang, "ingest.field.sku_desc"),
			Required:    false,
			ColumnIndex: idxSKU,
			ColumnName:  nameSKU,
		},
		{
			Field:       productmatch.FieldBarcode,
			Label:       i18n.T(lang, "ingest.field.barcode"),
			Description: i18n.T(lang, "ingest.field.barcode_desc"),
			Required:    false,
			ColumnIndex: idxBar,
			ColumnName:  nameBar,
		},
		{
			Field:       productmatch.FieldExpiryDate,
			Label:       i18n.T(lang, "ingest.field.expiry"),
			Description: i18n.T(lang, "ingest.field.expiry_desc"),
			Required:    false,
			ColumnIndex: idxExp,
			ColumnName:  nameExp,
		},
		{
			Field:       productmatch.FieldDiscountPct,
			Label:       i18n.T(lang, "ingest.field.discount"),
			Description: i18n.T(lang, "ingest.field.discount_desc"),
			Required:    false,
			ColumnIndex: idxDisc,
			ColumnName:  nameDisc,
		},
	}
}

// FieldChoices lists the fields a column may be bound to, grouped for the
// dropdown.
func FieldChoices() []productmatch.Spec { return productmatch.Specs }

// FieldGroups lists the review screen's sections in order.
func FieldGroups() []productmatch.Group { return productmatch.Groups }

// ConfidenceTone maps a confidence onto the badge palette.
func ConfidenceTone(c productmatch.Confidence) string {
	switch c {
	case productmatch.ConfidenceCertain:
		return "badge-emerald"
	case productmatch.ConfidenceHigh:
		return "badge-sky"
	case productmatch.ConfidenceMedium:
		return "badge-amber"
	case productmatch.ConfidenceLow:
		return "badge-rose"
	}
	return "badge-slate"
}

// SeverityTone maps a finding's severity onto the badge palette.
func SeverityTone(s productmatch.Severity) string {
	switch s {
	case productmatch.SeverityError:
		return "badge-rose"
	case productmatch.SeverityWarning:
		return "badge-amber"
	}
	return "badge-slate"
}

// OutcomeTone maps a row outcome onto the badge palette.
func OutcomeTone(outcome string) string {
	switch outcome {
	case ingest.OutcomeInserted:
		return "badge-emerald"
	case ingest.OutcomeUpdated:
		return "badge-sky"
	case ingest.OutcomeError:
		return "badge-rose"
	}
	return "badge-slate"
}

// OutcomeLabel renders a row outcome in the given language (defaults to Arabic).
func OutcomeLabel(outcome string, langOpt ...string) string {
	lang := "ar"
	if len(langOpt) > 0 && langOpt[0] != "" {
		lang = langOpt[0]
	}
	switch outcome {
	case ingest.OutcomeInserted:
		return i18n.T(lang, "ingest.outcome.inserted")
	case ingest.OutcomeUpdated:
		return i18n.T(lang, "ingest.outcome.updated")
	case ingest.OutcomeError:
		return i18n.T(lang, "ingest.outcome.error")
	}
	return i18n.T(lang, "ingest.outcome.skipped")
}

// MatchLevelLabel renders a match level in Arabic.
func MatchLevelLabel(level string) string {
	return productmatch.MatchLevel(level).Label()
}

// MatchLevelBadgeClass maps a match level to a badge tone.
func MatchLevelBadgeClass(level string) string {
	switch productmatch.MatchLevel(level) {
	case productmatch.MatchBarcode, productmatch.MatchCode, productmatch.MatchExact, productmatch.MatchStrong:
		return "badge-emerald"
	case productmatch.MatchReview:
		return "badge-amber"
	case productmatch.MatchAmbiguous:
		return "badge-purple"
	default:
		return "badge-slate"
	}
}

// PercentText renders a 0..1 score as a percentage.
func PercentText(score float64) string {
	return fmt.Sprintf("%d%%", int(score*100+0.5))
}

// FileSizeText renders a byte count the way a vendor reads it.
func FileSizeText(bytes int64, langOpt ...string) string {
	lang := "ar"
	if len(langOpt) > 0 && langOpt[0] != "" {
		lang = langOpt[0]
	}
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf(i18n.T(lang, "common.file_size_mb"), float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf(i18n.T(lang, "common.file_size_kb"), float64(bytes)/float64(1<<10))
	}
	return fmt.Sprintf(i18n.T(lang, "common.file_size_bytes"), bytes)
}

// StockModeOptions are the three readings of an imported quantity.
func StockModeOptions() []inventory.StockMode {
	return []inventory.StockMode{inventory.StockReplace, inventory.StockAdd, inventory.StockKeep}
}

// importRowsURL links one filter tab of the results table.
func importRowsURL(publicID, outcome string) string {
	if outcome == "" {
		return "/vendor/ingest/" + publicID
	}
	return "/vendor/ingest/" + publicID + "?outcome=" + outcome
}

// importWarehouseName names the chosen warehouse on the confirmation screen.
func importWarehouseName(view VendorImportView) string {
	if view.Session == nil {
		return ""
	}
	for _, wh := range view.Warehouses {
		if wh.ID == view.Session.Settings.WarehouseID {
			return wh.Name
		}
	}
	return "—"
}
