package ui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminAIMatchConfigsPage lists qualified and candidate AI match configurations (US10 T134).
// Protected by permission: platform.ai.configure.
func (h *UIHandler) AdminAIMatchConfigsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.catSvc == nil || h.catSvc.Matching() == nil {
		http.Error(w, "matching service unavailable", http.StatusServiceUnavailable)
		return
	}

	configs, err := h.catSvc.Matching().ListAIConfigurations(ctx)
	if err != nil {
		data := pages.AdminAIMatchConfigsData{
			ErrorMessage: fmt.Sprintf("تعذر تحميل الإعدادات: %v", err),
		}
		h.renderPage(ctx, w, "render AdminAIMatchConfigsPage", pages.AdminAIMatchConfigsPage(lang, dir, data))
		return
	}

	data := pages.AdminAIMatchConfigsData{
		Configs:    configs,
		SuccessMsg: r.URL.Query().Get("msg"),
	}
	h.renderPage(ctx, w, "render AdminAIMatchConfigsPage", pages.AdminAIMatchConfigsPage(lang, dir, data))
}

// AdminAIMatchConfigUpdateStatus changes an AI configuration's status with an audit reason (US10 T134).
// Protected by permission: platform.ai.configure.
func (h *UIHandler) AdminAIMatchConfigUpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configID := strings.TrimSpace(chi.URLParam(r, "id"))
	if configID == "" {
		http.Error(w, "missing configuration id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	newStatus := strings.TrimSpace(r.FormValue("status"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	if newStatus == "" || reason == "" {
		http.Redirect(w, r, "/admin/matching/ai-configs?msg="+httpErrorMessage("الحالة والسبب مطلوبان"), http.StatusSeeOther)
		return
	}

	changedBy := "system"
	if actor, ok := authctx.From(ctx); ok {
		if actor.Email != "" {
			changedBy = actor.Email
		} else if actor.UserID > 0 {
			changedBy = fmt.Sprintf("user:%d", actor.UserID)
		}
	}

	if h.catSvc == nil || h.catSvc.Matching() == nil {
		http.Error(w, "matching service unavailable", http.StatusServiceUnavailable)
		return
	}

	err := h.catSvc.Matching().UpdateAIConfigurationStatus(ctx, configID, newStatus, changedBy, reason)
	if err != nil {
		http.Redirect(w, r, "/admin/matching/ai-configs?msg="+httpErrorMessage(fmt.Sprintf("فشل التحديث: %v", err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/matching/ai-configs?msg="+httpSuccessMessage("تم تحديث حالة الإعداد بنجاح"), http.StatusSeeOther)
}

func httpErrorMessage(msg string) string {
	return msg
}

func httpSuccessMessage(msg string) string {
	return msg
}
