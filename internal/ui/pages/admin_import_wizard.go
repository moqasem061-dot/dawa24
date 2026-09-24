package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// ImportToggle is one switch on the configuration screen.
type ImportToggle struct {
	Name        string
	Icon        string
	Title       string
	Description string
	Checked     bool
	// Disabled marks a switch that cannot be used right now, with Note saying
	// why. The AI switch uses this when the Gateway is unreachable.
	Disabled bool
	Note     string
}

// ImportConfigureView is the upload screen.
type ImportConfigureView struct {
	Modes        []catalog.ImportModeOption
	SelectedMode catalog.ImportMode
	Toggles      []ImportToggle
	Categories   []catalog.TaxonomyOption
	AIAvailable  bool
	// Recent is the import history panel.
	Recent []*catalog.ImportSession
	// Fatal explains a rejected upload, if the admin has just had one.
	Fatal       string
	FatalDetail string
}

// NewImportConfigureView builds the upload screen from the platform's state.
func NewImportConfigureView(
	categories []catalog.TaxonomyOption, recent []*catalog.ImportSession, aiAvailable bool,
) ImportConfigureView {
	opts := catalog.DefaultImportOptions()
	return ImportConfigureView{
		Modes:        catalog.ImportModeOptions,
		SelectedMode: catalog.ModeUpdateAndAdd,
		Toggles:      importToggles(opts, aiAvailable),
		Categories:   categories,
		Recent:       recent,
		AIAvailable:  aiAvailable,
	}
}

// importToggles renders the enrichment switches in a fixed order.
func importToggles(opts catalog.ImportOptions, aiAvailable bool) []ImportToggle {
	toggles := []ImportToggle{
		{
			Name: "auto_create_brands", Icon: "🏭",
			Title:       i18n.T("ar", "wizard.opt.auto_create_mfr_title"),
			Description: i18n.T("ar", "wizard.opt.auto_create_mfr_desc"),
			Checked:     opts.AutoCreateBrands,
		},
		{
			Name: "assign_category", Icon: "🗂️",
			Title:       i18n.T("ar", "wizard.opt.assign_cat_title"),
			Description: i18n.T("ar", "wizard.opt.assign_cat_desc"),
			Checked:     opts.AssignCategory,
		},
		{
			Name: "auto_create_categories", Icon: "➕",
			Title:       i18n.T("ar", "wizard.opt.auto_create_cats_title"),
			Description: i18n.T("ar", "wizard.opt.auto_create_cats_desc"),
			Checked:     opts.AutoCreateCategories,
		},
		{
			Name: "assign_dosage_form", Icon: "💊",
			Title:       i18n.T("ar", "wizard.opt.assign_dosage_title"),
			Description: i18n.T("ar", "wizard.opt.assign_dosage_desc"),
			Checked:     opts.AssignDosageForm,
		},
		{
			Name: "assign_scientific_name", Icon: "🧪",
			Title:       i18n.T("ar", "wizard.opt.assign_scientific_title"),
			Description: i18n.T("ar", "wizard.opt.assign_scientific_desc"),
			Checked:     opts.AssignScientificName,
		},
		{
			Name: "merge_duplicate_rows", Icon: "🧲",
			Title:       "دمج الصفوف المكررة",
			Description: "يدمج الصفوف التي تصف نفس الصنف في صف واحد. الأصناف التي يختلف تركيزها أو شكلها الصيدلي تبقى منفصلة.",
			Checked:     opts.MergeDuplicateRows,
		},
		{
			Name: "download_images", Icon: "🖼️",
			Title:       "تنزيل ومعالجة صور المنتجات",
			Description: "يقوم بتنزيل الصور من الروابط، وتطبيق العلامة المائية وضغطها وحفظها في التخزين السحابي.",
			Checked:     opts.DownloadImages,
		},
	}

	// One line, not four paragraphs. The description used to explain the
	// request batching, the candidate shortlisting and the per-request row
	// count — accurate, and nothing an administrator deciding whether to tick a
	// box needs to read. It is a feature, not a whitepaper.
	ai := ImportToggle{
		Name: "use_ai", Icon: "🤖",
		Title:   i18n.T("ar", "wizard.opt.use_ai_title"),
		Checked: opts.UseAI && aiAvailable,
	}
	if !aiAvailable {
		ai.Disabled = true
		ai.Checked = false
		ai.Note = i18n.T("ar", "wizard.opt.use_ai_note_unavailable")
	}
	return append(toggles, ai)
}
