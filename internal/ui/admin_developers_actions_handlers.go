package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminGatewayTestConnection probes Gateway readiness live.
func (h *UIHandler) AdminGatewayTestConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json")

	_ = r.ParseForm()
	reqEndpoint := platformadmin.NormalizeGatewayEndpoint(r.FormValue("endpoint_url"))
	reqUser := strings.TrimSpace(r.FormValue("admin_username"))
	reqKey := strings.TrimSpace(r.FormValue("api_key"))

	// If key is empty or masked, fallback to stored credentials
	if h.adminSvc != nil {
		if gwSets, loadErr := h.adminSvc.GetGatewaySettings(ctx); loadErr != nil {
			h.logLoadErr(ctx, "adminSvc.GetGatewaySettings", loadErr)
		} else if gwSets != nil {
			savedUser, savedPass := gwSets.AdminCredentials()
			if (reqKey == "" || strings.HasPrefix(reqKey, "•")) && savedPass != "" {
				reqKey = savedPass
			}
			if reqUser == "" && savedUser != "" {
				reqUser = savedUser
			}
			if reqEndpoint == "" && gwSets.EndpointURL != "" {
				reqEndpoint = platformadmin.NormalizeGatewayEndpoint(gwSets.EndpointURL)
			}
		}
	}

	if reqEndpoint == "" {
		reqEndpoint = "https://api.dawa24.com"
	}
	if reqUser == "" {
		reqUser = "admin"
	}

	// A credential typed into the connection test travels to the Gateway host
	// exactly like a saved one, so it is validated on the same terms.
	if err := platformadmin.ValidateAdminCredential(reqKey); err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "rejected",
			"message": credentialRejectionMessage(err),
		})
		return
	}

	adminClient := gateway.NewAdminClient(reqEndpoint, reqUser, reqKey)
	plans, err := adminClient.ListPlans(ctx)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		errStr := err.Error()
		var msg string
		switch {
		case strings.Contains(errStr, "no such host"):
			msg = fmt.Sprintf("تعذر الاتصال بـ %s: خطأ في خادم الأسماء (DNS Lookup Failed: no such host). يرجى التأكد من كتابة الرابط بشكل صحيح وتحديث إعدادات DNS للخادم.", reqEndpoint)
		case strings.Contains(errStr, "connection refused"):
			msg = fmt.Sprintf("تعذر الاتصال بـ %s: تم رفض الاتصال بالخادم (Connection Refused). تأكد من تشغيل خدمة البوابة.", reqEndpoint)
		case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
			msg = fmt.Sprintf("تعذر الاتصال بـ %s: انتهت مهلة الاتصال بالبوابة (Timeout).", reqEndpoint)
		case strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized"):
			msg = fmt.Sprintf("بيانات الاعتماد غير مصرح بها (401 Unauthorized) للبوابة %s. تأكد من صحة اسم المستخدم وكلمة المرور.", reqEndpoint)
		case strings.Contains(errStr, "404") || strings.Contains(errStr, "Not Found"):
			msg = fmt.Sprintf("المسار غير موجود (404 Not Found) على %s. تأكد من أن الرابط هو الرابط الأساسي للبوابة دون إضافة /v1 أو /api.", reqEndpoint)
		default:
			msg = fmt.Sprintf(i18n.T(lang, "admin.dev.connection_failed_format"), reqEndpoint, err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "unreachable",
			"error":   err.Error(),
			"message": msg,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "healthy",
		"message": fmt.Sprintf(i18n.T(lang, "admin.dev.connection_healthy_format"), reqEndpoint, len(plans)),
		"count":   len(plans),
	})
}

// AdminErrorLogStatusSubmit updates the status of an error record.
func (h *UIHandler) AdminErrorLogStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", i18n.T(lang, "admin.dev.invalid_log_id"))
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "RESOLVED"
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	if err := h.adminSvc.UpdateErrorLogStatus(ctx, id, status); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "success", i18n.T(lang, "admin.dev.error_status_updated_success"))
}

// AdminErrorDetailFragment serves one error's full diagnostics into the modal.
//
// It replaces a per-row <script type="application/json"> block whose contents
// templ escaped, so JSON.parse threw and the modal rendered the Alpine handler's
// two-field fallback. Loading it here also stops a thirty-row page from
// carrying thirty stack traces it will never show.
func (h *UIHandler) AdminErrorDetailFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.renderPage(ctx, w, "render AdminErrorDetailFragment", pages.AdminErrorDetailFragment(nil, lang))
		return
	}

	entry, err := h.adminSvc.GetErrorLogByID(database.AsSystem(ctx), id)
	if err != nil {
		// A read failure is reported rather than swallowed into an empty modal,
		// which is the failure mode this endpoint exists to remove.
		h.log.ErrorContext(ctx, "load error log detail", "error_id", id, "error", err)
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := pages.AdminErrorDetailFragment(entry, lang).Render(ctx, w); renderErr != nil {
		h.log.ErrorContext(ctx, "render error log detail", "error", renderErr)
	}
}

// AdminAuditDetailFragment serves one audit entry's diff into its modal.
//
// Same defect, same remedy as AdminErrorDetailFragment: the diff was embedded
// as templ-escaped JSON and never parsed.
func (h *UIHandler) AdminAuditDetailFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.renderPage(ctx, w, "render AdminAuditDetailFragment", pages.AdminAuditDetailFragment(nil, lang))
		return
	}

	entry, err := h.adminSvc.GetAuditEntryByID(database.AsSystem(ctx), id)
	if err != nil {
		h.log.ErrorContext(ctx, "load audit entry detail", "audit_id", id, "error", err)
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := pages.AdminAuditDetailFragment(entry, lang).Render(ctx, w); renderErr != nil {
		h.log.ErrorContext(ctx, "render audit entry detail", "error", renderErr)
	}
}
