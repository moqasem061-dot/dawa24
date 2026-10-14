package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminMatchPoliciesPage displays policy assignments and registered policies (US13 T152).
// Protected by permission: catalog.matching.policy.
func (h *UIHandler) AdminMatchPoliciesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.catSvc == nil || h.catSvc.Matching() == nil {
		http.Error(w, "matching service unavailable", http.StatusServiceUnavailable)
		return
	}

	matchSvc := h.catSvc.Matching()

	policies, err := matchSvc.ListPolicies(ctx)
	if err != nil {
		data := pages.AdminMatchPoliciesData{
			ErrorMessage: fmt.Sprintf("تعذر تحميل السياسات: %v", err),
		}
		h.renderPage(ctx, w, "render AdminMatchPoliciesPage", pages.AdminMatchPoliciesPage(lang, dir, data))
		return
	}

	assignments, err := matchSvc.ListAssignments(ctx)
	if err != nil {
		data := pages.AdminMatchPoliciesData{
			ErrorMessage: fmt.Sprintf("تعذر تحميل التخصيصات: %v", err),
		}
		h.renderPage(ctx, w, "render AdminMatchPoliciesPage", pages.AdminMatchPoliciesPage(lang, dir, data))
		return
	}

	data := pages.AdminMatchPoliciesData{
		Policies:     policies,
		Assignments:  assignments,
		SuccessMsg:   r.URL.Query().Get("msg"),
		ErrorMessage: r.URL.Query().Get("err"),
	}
	h.renderPage(ctx, w, "render AdminMatchPoliciesPage", pages.AdminMatchPoliciesPage(lang, dir, data))
}

// AdminMatchPolicyUpdateAssignment updates or sets active and shadow policy for a workflow cohort (US13 T152).
// Protected by permission: catalog.matching.policy.
func (h *UIHandler) AdminMatchPolicyUpdateAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	workflow := strings.TrimSpace(r.FormValue("workflow"))
	cohort := strings.TrimSpace(r.FormValue("cohort"))
	activeVersion := strings.TrimSpace(r.FormValue("active_version"))
	shadowVersionRaw := strings.TrimSpace(r.FormValue("shadow_version"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	if workflow == "" || cohort == "" || activeVersion == "" || reason == "" {
		http.Redirect(w, r, "/admin/matching/policies?err="+url.QueryEscape("جميع الحقول بما فيها السبب مطلوبة لتوثيق التغيير"), http.StatusSeeOther)
		return
	}

	var shadowVersion *string
	if shadowVersionRaw != "" {
		shadowVersion = &shadowVersionRaw
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

	err := h.catSvc.Matching().UpdateAssignment(ctx, workflow, cohort, activeVersion, shadowVersion, changedBy, reason)
	if err != nil {
		http.Redirect(w, r, "/admin/matching/policies?err="+url.QueryEscape(fmt.Sprintf("فشل تحديث التخصيص: %v", err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/matching/policies?msg="+url.QueryEscape("تم تحديث تخصيص السياسة بنجاح"), http.StatusSeeOther)
}

// AdminMatchPolicyRollback performs a one-click rollback to the previous policy version (US13 T152).
// Protected by permission: catalog.matching.policy.
func (h *UIHandler) AdminMatchPolicyRollback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	workflow := strings.TrimSpace(r.FormValue("workflow"))
	cohort := strings.TrimSpace(r.FormValue("cohort"))

	if workflow == "" || cohort == "" {
		http.Redirect(w, r, "/admin/matching/policies?err="+url.QueryEscape("المسار والشريحة مطلوبان للتراجع"), http.StatusSeeOther)
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

	err := h.catSvc.Matching().RollbackAssignment(ctx, workflow, cohort, changedBy)
	if err != nil {
		http.Redirect(w, r, "/admin/matching/policies?err="+url.QueryEscape(fmt.Sprintf("فشل استعادة الإصدار السابق: %v", err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/matching/policies?msg="+url.QueryEscape(fmt.Sprintf("تم استعادة الإصدار السابق للمسار %s بنجاح", workflow)), http.StatusSeeOther)
}
