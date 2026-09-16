package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminGatewayTestConnection probes Gateway readiness live.
func (h *UIHandler) AdminGatewayTestConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json")

	_ = r.ParseForm()
	reqEndpoint := strings.TrimSpace(r.FormValue("endpoint_url"))
	reqUser := strings.TrimSpace(r.FormValue("admin_username"))
	if reqUser == "" {
		reqUser = "admin"
	}
	reqKey := strings.TrimSpace(r.FormValue("api_key"))

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

	adminClient, endpoint, _ := h.getGatewayAdminClient(ctx)
	if reqEndpoint != "" {
		endpoint = reqEndpoint
		adminClient = gateway.NewAdminClient(reqEndpoint, reqUser, reqKey)
	}

	plans, err := adminClient.ListPlans(ctx)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		msg := fmt.Sprintf(i18n.T(lang, "admin.dev.connection_failed_format"), endpoint, err)
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			msg = fmt.Sprintf(i18n.T(lang, "admin.dev.unauthorized_format"), endpoint)
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
		"message": fmt.Sprintf(i18n.T(lang, "admin.dev.connection_healthy_format"), endpoint, len(plans)),
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
		_ = pages.AdminErrorDetailFragment(nil, lang).Render(ctx, w)
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
		_ = pages.AdminAuditDetailFragment(nil, lang).Render(ctx, w)
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
