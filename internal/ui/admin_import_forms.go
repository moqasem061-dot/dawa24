package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Reading the wizard's forms, and the upload itself.
//
// Kept apart from the handlers because these are the parts with rules of their
// own: what an empty box means as against an explicit zero, what the body cap
// actually caps, and which of two field names a given screen posts under.

// importPath builds a URL on one import session. verb of "" is the review page.
func importPath(publicID, verb string) string {
	if verb == "" {
		return "/admin/products/import/" + publicID
	}
	return "/admin/products/import/" + publicID + "/" + verb
}

// readImportSettings reads the strategy, the switches, and the column
// corrections out of a submitted form.
func readImportSettings(r *http.Request) catalog.ImportSettings {
	return catalog.ImportSettings{
		Mode: catalog.ParseMode(r.PostFormValue("import_mode")),
		Options: catalog.ImportOptions{
			AutoCreateBrands:     formChecked(r, "auto_create_brands"),
			AssignCategory:       formChecked(r, "assign_category"),
			AutoCreateCategories: formChecked(r, "auto_create_categories"),
			AssignDosageForm:     formChecked(r, "assign_dosage_form"),
			AssignScientificName: formChecked(r, "assign_scientific_name"),
			UseAI:                formChecked(r, "use_ai"),
			DefaultCategoryID:    formInt64(r, "default_category_id"),
			MinMatchScore:        formMatchScore(r, "min_match_score"),
		}.Normalize(),
		Overrides: readLayoutOverrides(r),
	}
}

// readLayoutOverrides reads the admin's corrections to the detected structure.
//
// The mapping screen posts a chooser for every field, so a field left on "do
// not read" arrives as an explicit zero and is recorded as such. That is a
// different instruction from silence: an omitted field would be re-detected on
// the next run, quietly re-binding a column the admin deliberately unbound.
func readLayoutOverrides(r *http.Request) catalog.LayoutOverrides {
	overrides := catalog.LayoutOverrides{
		HeaderRow:    int(formInt64(r, "header_row")),
		FirstDataRow: int(formInt64(r, "first_data_row")),
		LastDataRow:  int(formInt64(r, "last_data_row")),
		Columns:      map[string]int{},
	}

	for key, values := range r.PostForm {
		field, isColumn := strings.CutPrefix(key, "col_")
		if !isColumn || len(values) == 0 {
			continue
		}
		raw := strings.TrimSpace(values[0])
		if raw == "" {
			continue // untouched: leave the detection alone
		}
		column, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if column <= 0 {
			overrides.Columns[field] = catalog.IgnoreColumn
			continue
		}
		overrides.Columns[field] = column
	}

	if len(overrides.Columns) == 0 {
		overrides.Columns = nil
	}
	return overrides
}

func formChecked(r *http.Request, name string) bool {
	value := r.PostFormValue(name)
	return value == "1" || value == "on" || value == "true"
}

func formInt64(r *http.Request, name string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue(name)), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// querySuffix carries the review table's filter and page across a redirect.
func querySuffix(r *http.Request) string {
	if raw := r.URL.RawQuery; raw != "" {
		return "?" + raw
	}
	return ""
}

// actorUserID is the signed-in admin, or zero when the request has no actor.
func actorUserID(ctx context.Context) int64 {
	if actor, ok := authctx.From(ctx); ok {
		return actor.UserID
	}
	return 0
}

// aiAvailable reports whether the AI switch can be offered. It is off by
// default whether or not the Gateway answers; this only decides whether the
// admin is allowed to turn it on.
func (h *UIHandler) aiAvailable(ctx context.Context) bool {
	return h.catSvc != nil && h.catSvc.AIAvailable(ctx)
}

// importCategories is the taxonomy offered in the wizard's category chooser.
func (h *UIHandler) importCategories(ctx context.Context) []catalog.TaxonomyOption {
	if h.catSvc == nil {
		return nil
	}
	vocab, err := h.catSvc.ImportVocabulary(database.AsSystem(ctx), 0)
	if err != nil {
		h.log.DebugContext(ctx, "import categories unavailable", "error", err)
		return nil
	}
	return vocab.Categories
}

// recentImportSessions backs the history panel on the upload screen.
func (h *UIHandler) recentImportSessions(ctx context.Context) []*catalog.ImportSession {
	if h.catSvc == nil {
		return nil
	}
	sessions, err := h.catSvc.RecentImportSessions(database.AsSystem(ctx), 0, 8)
	if err != nil {
		h.log.DebugContext(ctx, "import history unavailable", "error", err)
		return nil
	}
	return sessions
}

// importMessage prefers the domain's own localized message over a raw error.
func (h *UIHandler) importMessage(err error, r *http.Request) string {
	lang := langOf(r)
	if msg := h.safeMessage(err, lang); msg != "" && msg != i18n.T(lang, "notice.error_generic") && msg != i18n.T(lang, "errors.server_error") {
		return msg
	}
	return err.Error()
}

// uploadError carries an admin-facing reason alongside the technical detail,
// which is shown smaller and logged rather than being the headline.
type uploadError struct {
	message string
	detail  string
}

func (e *uploadError) Error() string { return e.message }

// readUploadedFile pulls the spreadsheet out of the multipart request.
//
// The field is accepted under two names because two different screens post here
// — the import wizard sends "import_file" and the older warehouse upload form
// sends "file".
func readUploadedFile(r *http.Request) ([]byte, string, *uploadError) {
	lang := langOf(r)
	// The body cap is enforced here rather than trusting the multipart parser's
	// memory limit, which bounds what is held in RAM, not what a client may
	// stream. A w of nil only means "cannot flag the connection as too large";
	// the read itself still fails past the cap.
	r.Body = http.MaxBytesReader(nil, r.Body, maxImportRequestBytes)

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		return nil, "", &uploadError{
			message: fmt.Sprintf(i18n.T(lang, "admin.import.upload_max_bytes_format"),
				maxImportUploadBytes>>20),
			detail: err.Error(),
		}
	}

	file, header, err := r.FormFile("import_file")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		return nil, "", &uploadError{
			message: i18n.T(lang, "admin.import.no_file_selected"),
		}
	}
	defer func() { _ = file.Close() }()

	// One byte past the cap, so a file exactly at the limit still reads whole
	// and anything larger is detectable without buffering all of it.
	content, err := io.ReadAll(io.LimitReader(file, maxImportUploadBytes+1))
	if err != nil {
		return nil, "", &uploadError{
			message: i18n.T(lang, "admin.import.read_content_failed"),
			detail:  err.Error(),
		}
	}
	if int64(len(content)) > maxImportUploadBytes {
		return nil, "", &uploadError{
			message: fmt.Sprintf(i18n.T(lang, "admin.import.file_too_large_format"),
				maxImportUploadBytes>>20),
		}
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	if err := filesecurity.ValidateSpreadsheetSecurity(content, filename); err != nil {
		return nil, "", &uploadError{
			message: filesecurity.SecurityErrorMessage,
			detail:  err.Error(),
		}
	}
	return content, filename, nil
}

// formMatchScore reads the shared "أقل نسبة مطابقة" control, which every import
// screen renders as a whole-number percentage. Zero means "use the platform
// default" and Normalize supplies it.
func formMatchScore(r *http.Request, name string) float64 {
	raw := strings.TrimSpace(r.PostFormValue(name))
	if raw == "" {
		return 0
	}
	pct, err := strconv.ParseFloat(raw, 64)
	if err != nil || pct <= 0 {
		return 0
	}
	if pct > 1 {
		pct /= 100
	}
	return pct
}
