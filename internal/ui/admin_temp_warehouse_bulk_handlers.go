package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminTempWarehouseBulkSubmit processes bulk actions (archive, unarchive, delete) on selected warehouses.
func (h *UIHandler) AdminTempWarehouseBulkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", i18n.T(lang, "admin.temp_wh.invalid_data"))
		return
	}

	action := strings.TrimSpace(r.PostFormValue("bulk_action"))
	idsRaw := r.PostForm["selected_ids"]
	if len(idsRaw) == 0 {
		if raw := strings.TrimSpace(r.PostFormValue("selected_ids")); raw != "" {
			idsRaw = strings.Split(raw, ",")
		}
	}

	var ids []int64
	for _, s := range idsRaw {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", "لم يتم تحديد أي مستودعات لتنفيذ الإجراء")
		return
	}

	var affected int64
	var opErr error

	switch action {
	case "archive":
		reason := i18n.T(lang, "admin.temp_wh.manual_archive_reason")
		affected, opErr = h.compareSvc.BulkArchiveFiles(database.AsSystem(ctx), ids, nil, reason)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", "لم يتم العثور على مستودعات قابلة للأرشفة أو حدث خطأ أثناء التنفيذ")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "success", fmt.Sprintf("تم أرشفة %d مستودع بنجاح", affected))
	case "unarchive":
		affected, opErr = h.compareSvc.BulkUnarchiveFiles(database.AsSystem(ctx), ids, nil)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", "لم يتم العثور على مستودعات قابلة للتفعيل أو حدث خطأ أثناء التنفيذ")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "success", fmt.Sprintf("تم تفعيل واسترجاع %d مستودع بنجاح", affected))
	case "delete":
		affected, opErr = h.compareSvc.BulkDeleteFiles(database.AsSystem(ctx), ids, nil)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", "لم يتم العثور على المستودعات المحددة لحذفها أو قد تم حذفها مسبقاً")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "success", fmt.Sprintf("تم حذف %d مستودع وكافة أصنافها نهائياً", affected))
	default:
		h.redirectWithNotice(w, r, tempWarehouseSuperPage, "error", "إجراء غير معروف")
	}
}
