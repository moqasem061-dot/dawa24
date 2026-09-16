package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// maxUploadBytes caps a direct upload.
//
// 64 MB. A ten-thousand-row workbook of the shape a pharmacy actually sends is
// around 250 KB, so this is not a row-count limit in disguise — it is a guard
// against a mis-selected file. The previous 20 MB was low enough that a workbook
// carrying embedded images (which pharmacy exports routinely do) was refused,
// and the refusal was reported as if the file were unreadable.
const maxUploadBytes = 64 << 20

// SmartOrderNewPage renders step 1.
func (h *UIHandler) SmartOrderNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/smart-order/new", http.StatusSeeOther)
		return
	}

	data := pages.SmartOrderNewData{Error: r.URL.Query().Get("error")}
	selectedBranchID := h.buyingBranchID(ctx, &actor)

	if h.orgSvc != nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err == nil {
			for _, b := range branches {
				data.Branches = append(data.Branches, pages.SmartOrderBranch{
					ID:     b.ID,
					Name:   b.Name.Get(i18n.Lang(lang)),
					City:   b.Address,
					IsMain: b.ID == selectedBranchID,
				})
			}
		}
	}

	if h.smartOrderSvc != nil {
		if p, err := h.smartOrderSvc.Profile(ctx, actor.OrganizationID); err == nil {
			data.Profile = p
		}
	}

	// The AI toggle is only offered when it would actually work. Rendering it
	// enabled against an unreachable Gateway produces a run that silently does
	// no AI at all, which reads as the feature being broken.
	data.AIAvailable, data.AIUnavailableReason = h.smartOrderAIState(ctx, actor.OrganizationID, lang)

	h.renderPage(ctx, w, "render smart order import page", pages.SmartOrderNewPage(lang, dir, data))
}

// SmartOrderCreateSubmit handles the upload and creates the run.
func (h *UIHandler) SmartOrderCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	if h.smartOrderSvc == nil {
		http.Error(w, "smart ordering is not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	// The in-memory portion is capped well below the body limit so a large
	// upload spills to a temp file instead of being held whole in RAM.
	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.upload_size_limit"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.file_required"))
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.empty_file"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(content, header.Filename); err != nil {
		h.smartOrderFail(w, r, filesecurity.SecurityErrorMessage)
		return
	}

	// Inspect before creating anything: an unreadable file should not leave a
	// half-formed run in the buyer's history.
	parsed, err := pipeline.Inspect(content, header.Filename)
	if err != nil {
		if strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			h.smartOrderFail(w, r, filesecurity.SecurityErrorMessage)
			return
		}
		// The reader's own message names the cause — empty file, unsupported
		// container, no readable sheet — and is already in Arabic. Passing it
		// through beats a generic "could not read the file", which tells the
		// buyer nothing they can act on.
		h.log.WarnContext(ctx, "could not read the uploaded file",
			"filename", header.Filename, "bytes", len(content), "error", err)
		h.smartOrderFail(w, r, fmt.Sprintf(i18n.T(lang, "smartorder.read_file_failed_format"), err.Error()))
		return
	}

	// The shell selector is authoritative across the entire buying surface.
	// Do not let a hand-posted form switch the branch behind the selector.
	branchID := h.buyingBranchID(ctx, &actor)
	if msg := h.smartOrderBranchRefusal(ctx, actor.OrganizationID, branchID, lang); msg != "" {
		h.smartOrderFail(w, r, msg)
		return
	}
	tolerance, _ := strconv.ParseFloat(r.FormValue("tolerance_pct"), 64)
	defaultQty, _ := strconv.Atoi(r.FormValue("default_quantity"))
	if defaultQty <= 0 {
		defaultQty = 1
	}

	var budget *money.Amount
	if s := strings.TrimSpace(r.FormValue("max_budget")); s != "" {
		if amt, err := money.Parse(s); err == nil {
			budget = &amt
		}
	}

	run, err := h.smartOrderSvc.Start(ctx, smartorder.StartOptions{
		UserID:            actor.UserID,
		OrganizationID:    actor.OrganizationID,
		BranchID:          branchID,
		Filename:          header.Filename,
		Criteria:          orderedCriteria(r),
		TolerancePct:      tolerance,
		DefaultQuantity:   defaultQty,
		MaxBudget:         budget,
		UseSavingProducts: r.FormValue("use_saving_products") != "",
		UseAIMatching:     r.FormValue("use_ai_matching") != "",
		MinMatchScore:     formMatchScore(r, "min_match_score"),
		MatchLanguage:     strings.TrimSpace(r.FormValue("match_language")),
	})
	if err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err, lang))
		return
	}

	// The file is held for the mapping step, against the run in the database.
	// It used to live in process memory, which lost it on every redeploy and on
	// any request that landed on a second instance — and asked the pharmacy to
	// upload a nine-thousand-line workbook again for no visible reason.
	if err := h.smartOrderSvc.SaveFile(ctx, run.ID, run.OrganizationID, header.Filename, content); err != nil {
		h.log.ErrorContext(ctx, "could not store the uploaded file", "run_id", run.ID, "error", err)
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.save_file_failed"))
		return
	}
	_ = parsed

	http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/mapping", http.StatusSeeOther)
}

// orderedCriteria reads the enabled criteria in the priority the buyer set.
//
// The form posts a checkbox and a priority number per criterion, because a
// drag-ordered list degrades badly without JavaScript and this screen must work
// without it.
func orderedCriteria(r *http.Request) []smartorder.Criterion {
	type entry struct {
		key      smartorder.Criterion
		priority int
	}
	var entries []entry
	for _, key := range r.Form["criteria"] {
		p, err := strconv.Atoi(r.FormValue("priority_" + key))
		if err != nil {
			p = 99
		}
		entries = append(entries, entry{key: smartorder.Criterion(key), priority: p})
	}
	// Insertion sort: at most three entries.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].priority < entries[j-1].priority; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	out := make([]smartorder.Criterion, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.key)
	}
	return out
}

// SmartOrderMappingPage renders step 2.
func (h *UIHandler) SmartOrderMappingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	content, filename, err := h.smartOrderSvc.File(ctx, run.ID, run.OrganizationID)
	if err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.file_no_longer_available"))
		return
	}

	parsed, err := pipeline.Inspect(content, filename)
	if err != nil {
		h.smartOrderFail(w, r, fmt.Sprintf(i18n.T(lang, "smartorder.read_file_failed_format"), err.Error()))
		return
	}

	data := pages.SmartOrderMappingData{
		Run:       run,
		Headers:   parsed.Headers,
		Preview:   parsed.Preview,
		HeaderRow: parsed.HeaderRow,
	}
	for _, f := range pipeline.TargetFields {
		field := pages.SmartOrderMappingField{
			Key: f.Key, Label: f.LabelAR, Required: f.Required,
			Confidence: parsed.Confidence[f.Key],
		}
		for col, assigned := range parsed.Detected {
			if assigned == f.Key {
				field.Column, field.Assigned = col, true
				break
			}
		}
		data.Fields = append(data.Fields, field)
	}

	h.renderPage(ctx, w, "render smart order mapping page", pages.SmartOrderMappingPage(lang, dir, data))
}

// SmartOrderMappingSubmit stages the rows and queues the run.
func (h *UIHandler) SmartOrderMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.form_parse_error"))
		return
	}

	content, filename, err := h.smartOrderSvc.File(ctx, run.ID, run.OrganizationID)
	if err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.file_no_longer_available"))
		return
	}

	headerRow, _ := strconv.Atoi(r.FormValue("header_row"))
	fields := make(map[int]string)
	for _, f := range pipeline.TargetFields {
		v := strings.TrimSpace(r.FormValue("mapping_" + f.Key))
		if v == "" {
			continue
		}
		if col, err := strconv.Atoi(v); err == nil {
			fields[col] = f.Key
		}
	}

	m := &smartorder.Mapping{HeaderRow: headerRow, Fields: fields, UserOverridden: true}
	if err := h.smartOrderSvc.ConfirmMapping(ctx, run, m); err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err, lang))
		return
	}

	lines, err := pipeline.Stage(content, filename, m, run.ID, run.OrganizationID)
	if err != nil {
		h.smartOrderFail(w, r, fmt.Sprintf(i18n.T(lang, "smartorder.stage_lines_failed_format"), err.Error()))
		return
	}
	if len(lines) == 0 {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.no_items_found"))
		return
	}
	if err := h.smartOrderSvc.StageLines(ctx, lines); err != nil {
		h.smartOrderFail(w, r, i18n.T(lang, "smartorder.save_lines_failed"))
		return
	}
	if err := h.smartOrderSvc.Queue(ctx, run); err != nil {
		h.smartOrderFail(w, r, translateSmartOrderError(err, lang))
		return
	}
	if h.smartOrderEnqueue != nil {
		if err := h.smartOrderEnqueue(ctx, run.ID, run.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "could not queue smart order run", "run_id", run.ID, "error", err)
		}
	}

	// The file has served its purpose the moment the rows exist.
	if err := h.smartOrderSvc.DropFile(ctx, run.ID, run.OrganizationID); err != nil {
		h.log.WarnContext(ctx, "could not drop the uploaded file", "run_id", run.ID, "error", err)
	}
	http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/progress", http.StatusSeeOther)
}

// smartOrderRun resolves the run in the URL for the caller's organisation.
func (h *UIHandler) smartOrderRun(w http.ResponseWriter, r *http.Request) (*smartorder.Run, bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return nil, false
	}
	if h.smartOrderSvc == nil {
		http.Error(w, "smart ordering is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	run, err := h.smartOrderSvc.Get(ctx, actor.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		// Not Found rather than Forbidden: a 403 would confirm the run exists.
		http.NotFound(w, r)
		return nil, false
	}
	return run, true
}

// smartOrderFail sends the buyer back with an explanation they can read.
//
// The message is URL-encoded, which it previously was not: an Arabic sentence
// dropped raw into a query string produces a mangled address and, on some
// proxies, no redirect at all. It is then rendered on the destination page —
// also new. Before this the only place the reason appeared was the address bar,
// which is where the buyer reported finding it.
func (h *UIHandler) smartOrderFail(w http.ResponseWriter, r *http.Request, message string) {
	h.log.WarnContext(r.Context(), "smart order step refused", "message", message)
	http.Redirect(w, r,
		"/customer/smart-order/new?error="+url.QueryEscape(message),
		http.StatusSeeOther)
}
