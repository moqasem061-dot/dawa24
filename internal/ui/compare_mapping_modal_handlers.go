package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareFileMappingModal renders the interactive modal HTML fragment for column mapping and setup mode.
func (h *UIHandler) CompareFileMappingModal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, i18n.T(lang, "compare.file.invalid_id"), http.StatusBadRequest)
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			http.Error(w, i18n.T(lang, "compare.file.not_found"), http.StatusNotFound)
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		http.Error(w, i18n.T(lang, "compare.file.access_forbidden"), http.StatusForbidden)
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(ctx, file)

	fieldMapping, scores, confidence := compare.DetectColumnsWithConfidence(headers)
	colMapping := make(map[compare.TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}

	detectedMapping := &compare.ColumnDetection{
		NameCol:     colMapping[compare.FieldProductName],
		PriceCol:    colMapping[compare.FieldPrice],
		DiscountCol: colMapping[compare.FieldDiscount],
		CodeCol:     colMapping[compare.FieldSKU],
		Confidence:  confidence,
		FieldScores: scores,
	}

	// Parse optional setup mode query parameters
	isSetup := r.URL.Query().Get("setup") == "1" || r.URL.Query().Get("setup_queue") != "" || r.URL.Query().Get("queue") != ""
	queueParam := strings.TrimSpace(r.URL.Query().Get("setup_queue"))
	if queueParam == "" {
		queueParam = strings.TrimSpace(r.URL.Query().Get("queue"))
	}
	step, _ := strconv.Atoi(r.URL.Query().Get("setup_step"))
	if step <= 0 {
		step, _ = strconv.Atoi(r.URL.Query().Get("step"))
	}
	if step <= 0 {
		step = 1
	}
	total, _ := strconv.Atoi(r.URL.Query().Get("setup_total"))
	if total <= 0 {
		total, _ = strconv.Atoi(r.URL.Query().Get("total"))
	}

	nextFileID, remainingQueue := parseCompareQueue(queueParam, id)
	if total <= 0 {
		if queueParam != "" {
			total = len(strings.Split(queueParam, ","))
		}
		if total <= 0 {
			total = 1
		}
	}

	h.renderPage(ctx, w, "render compare file mapping modal", pages.CompareFileMappingModal(file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID))
}

// CompareFileMappingPage shows the column mapping page or modal.
func (h *UIHandler) CompareFileMappingPage(w http.ResponseWriter, r *http.Request) {
	if h.isHTMX(r) || r.URL.Query().Get("modal") == "1" {
		h.CompareFileMappingModal(w, r)
		return
	}

	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.access_forbidden"))
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(ctx, file)

	fieldMapping, scores, confidence := compare.DetectColumnsWithConfidence(headers)
	colMapping := make(map[compare.TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}

	detectedMapping := &compare.ColumnDetection{
		NameCol:     colMapping[compare.FieldProductName],
		PriceCol:    colMapping[compare.FieldPrice],
		DiscountCol: colMapping[compare.FieldDiscount],
		CodeCol:     colMapping[compare.FieldSKU],
		Confidence:  confidence,
		FieldScores: scores,
	}

	isSetup := r.URL.Query().Get("setup") == "1" || r.URL.Query().Get("setup_queue") != ""
	queueParam := strings.TrimSpace(r.URL.Query().Get("setup_queue"))
	step, _ := strconv.Atoi(r.URL.Query().Get("setup_step"))
	if step <= 0 {
		step = 1
	}
	total, _ := strconv.Atoi(r.URL.Query().Get("setup_total"))
	if total <= 0 {
		total = 1
	}

	var nextFileID int64
	var remainingQueue string
	if queueParam != "" {
		idParts := strings.Split(queueParam, ",")
		var cleanedParts []string
		foundCurrent := false
		for _, part := range idParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			partID, _ := strconv.ParseInt(part, 10, 64)
			if partID == id {
				foundCurrent = true
				continue
			}
			if foundCurrent {
				if nextFileID == 0 {
					nextFileID = partID
				}
				cleanedParts = append(cleanedParts, part)
			}
		}
		if len(cleanedParts) > 0 {
			remainingQueue = strings.Join(cleanedParts, ",")
		}
	}

	h.renderPage(ctx, w, "render compare file mapping", pages.CompareFileMappingPage(lang, dir, file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID))
}
