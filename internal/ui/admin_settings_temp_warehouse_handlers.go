package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AdminTempWarehouseLifecycleSubmit persists the auto-archive and auto-delete settings for temp warehouses & compare files.
func (h *UIHandler) AdminTempWarehouseLifecycleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	autoArchiveHours, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("auto_archive_hours")))
	if err != nil || autoArchiveHours < 1 {
		autoArchiveHours = 720
	}

	autoDeleteDays, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("auto_delete_days")))
	if err != nil || autoDeleteDays < 1 {
		autoDeleteDays = 30
	}

	autoArchiveEnabled := r.PostFormValue("auto_archive_enabled") == "true" || r.PostFormValue("auto_archive_enabled") == "on" || r.PostFormValue("auto_archive_enabled") == "1"
	autoDeleteEnabled := r.PostFormValue("auto_delete_enabled") == "true" || r.PostFormValue("auto_delete_enabled") == "on" || r.PostFormValue("auto_delete_enabled") == "1"

	settings := &platformadmin.TempWarehouseLifecycleSettings{
		AutoArchiveHours:   autoArchiveHours,
		AutoArchiveEnabled: autoArchiveEnabled,
		AutoDeleteDays:     autoDeleteDays,
		AutoDeleteEnabled:  autoDeleteEnabled,
	}

	if err := h.adminSvc.SaveTempWarehouseLifecycleSettings(ctx, settings); err != nil {
		h.log.ErrorContext(ctx, "save temp warehouse lifecycle settings", "error", err)
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", h.safeMessage(err, lang))
		return
	}

	h.log.InfoContext(ctx, "temp warehouse lifecycle settings updated",
		"auto_archive_hours", autoArchiveHours,
		"auto_archive_enabled", autoArchiveEnabled,
		"auto_delete_days", autoDeleteDays,
		"auto_delete_enabled", autoDeleteEnabled,
	)
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", "تم حفظ إعدادات دورة حياة وأرشفة المستودعات المؤقتة وملفات المقارنة بنجاح.")
}

// AdminTempWarehouseRunLifecycle runs the lifecycle cleanup pass immediately on demand.
func (h *UIHandler) AdminTempWarehouseRunLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.compareSvc == nil || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	settings, err := h.adminSvc.GetTempWarehouseLifecycleSettings(ctx)
	if err != nil || settings == nil {
		settings = &platformadmin.TempWarehouseLifecycleSettings{
			AutoArchiveHours:   720,
			AutoArchiveEnabled: true,
			AutoDeleteDays:     30,
			AutoDeleteEnabled:  true,
		}
	}

	res, err := h.compareSvc.RunWarehouseLifecycle(
		ctx,
		settings.AutoArchiveHours,
		settings.AutoArchiveEnabled,
		settings.AutoDeleteDays,
		settings.AutoDeleteEnabled,
		storage.UploadBaseDir(),
	)
	if err != nil {
		h.log.ErrorContext(ctx, "manual run warehouse lifecycle failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "فشلت معالجة دورة حياة المستودعات: "+err.Error())
		return
	}

	msg := fmt.Sprintf("اكتملت دورة المعالجة بنجاح: تمت أرشفة %d مستودع/ملف، وحذف %d ملف نهائياً، وتفريغ %d سجل من قاعدة البيانات.",
		res.ArchivedFilesCount, res.DeletedFilesCount, res.PurgedRowsCount)
	h.log.InfoContext(ctx, "manual run warehouse lifecycle completed",
		"archived", res.ArchivedFilesCount,
		"deleted", res.DeletedFilesCount,
		"purged_rows", res.PurgedRowsCount,
	)
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", msg)
}
