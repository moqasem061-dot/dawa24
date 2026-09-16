package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The "مستودعاتي المرفوعة" screen: the same interface as the Super Admin
// temporary warehouses page, restricted to the signed-in staff member's own
// uploads (compare.files.user_id = me). Gated by inventory.my_temp_warehouse.*.
const (
	tempWarehouseMineBase = "/admin/my/temparte-warehouses"
)

// AdminMyTempWarehousesPage lists only the current user's uploaded temporary warehouses.
func (h *UIHandler) AdminMyTempWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	uid := currentActorUserID(r)
	filter := compare.AdminTempWarehouseFilter{
		Search:    strings.TrimSpace(r.URL.Query().Get("q")),
		OwnerOnly: &uid,
		SortBy:    strings.TrimSpace(r.URL.Query().Get("sort")),
		SortOrder: strings.TrimSpace(r.URL.Query().Get("order")),
	}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		st := compare.CompareFileStatus(s)
		filter.Status = &st
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	data := h.buildTempWarehousesData(ctx, filter, true, page, limit)
	data.Base = tempWarehouseMineBase
	data.PageURL = tempWarehouseMineBase
	if data.ActiveRun != nil {
		data.ActiveRun.BaseURL = tempWarehouseMineBase
	}
	data.NoticeMsg = strings.TrimSpace(r.URL.Query().Get("notice"))
	data.NoticeType = strings.TrimSpace(r.URL.Query().Get("notice_type"))
	if data.NoticeMsg == "" {
		if m := strings.TrimSpace(r.URL.Query().Get("msg")); m != "" {
			data.NoticeMsg = m
		}
	}

	h.renderPage(ctx, w, "render my temp warehouses", pages.AdminTempWarehousesPage(data, lang, dir))
}

// myTempWarehouseOwned parses {id} and returns the file if it belongs to the
// current user. Otherwise it writes a 403 (JSON or redirect) and returns false.
func (h *UIHandler) myTempWarehouseOwned(w http.ResponseWriter, r *http.Request) (*compare.CompareFile, bool) {
	ctx := r.Context()
	lang := langOf(r)
	fileID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || fileID <= 0 {
		h.myTempWarehouseReject(w, r, http.StatusBadRequest, i18n.T(lang, "admin.temp_wh.invalid_id"))
		return nil, false
	}
	f, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || f == nil {
		h.myTempWarehouseReject(w, r, http.StatusNotFound, i18n.T(lang, "admin.temp_wh.not_found"))
		return nil, false
	}
	if f.UserID != currentActorUserID(r) {
		h.myTempWarehouseReject(w, r, http.StatusForbidden, i18n.T(lang, "admin.temp_wh.not_owner"))
		return nil, false
	}
	return f, true
}

func (h *UIHandler) myTempWarehouseReject(w http.ResponseWriter, r *http.Request, code int, msg string) {
	if isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
		return
	}
	h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", msg)
}

// AdminMyTempWarehouseItemsJSON — read the rows of an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseItemsJSON(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.myTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseItemsJSON(w, r)
}

// AdminMyTempWarehouseMappingJSON — read column mapping of an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseMappingJSON(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.myTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseMappingJSON(w, r)
}

// AdminMyTempWarehouseExportXLSX — export an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseExportXLSX(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.myTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseExportXLSX(w, r)
}

// AdminMyTempWarehouseMappingSubmit — update column mapping of an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseMappingSubmit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.myTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseMappingSubmit(w, r)
}

// AdminMyTempWarehouseDeleteSubmit — delete an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	f, ok := h.myTempWarehouseOwned(w, r)
	if !ok {
		return
	}
	if err := h.compareSvc.DeleteFile(database.AsSystem(ctx), f.ID); err != nil {
		h.log.ErrorContext(ctx, "my temp warehouse delete", "error", err, "file_id", f.ID)
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", i18n.T(lang, "admin.temp_wh.deleted_success"))
}

// AdminMyTempWarehouseToggleArchiveSubmit — archive / unarchive an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseToggleArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	f, ok := h.myTempWarehouseOwned(w, r)
	if !ok {
		return
	}
	if f.Status == compare.FileArchived {
		if err := h.compareSvc.UnarchiveFile(database.AsSystem(ctx), f.ID); err != nil {
			h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", h.safeMessage(err, lang))
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", i18n.T(lang, "admin.temp_wh.unarchived_success"))
		return
	}
	if err := h.compareSvc.ArchiveFile(database.AsSystem(ctx), f.ID, i18n.T(lang, "admin.temp_wh.manual_archive_reason")); err != nil {
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", i18n.T(lang, "admin.temp_wh.archived_success"))
}

// AdminMyTempWarehouseItemDeleteSubmit — delete one row from an owned warehouse.
func (h *UIHandler) AdminMyTempWarehouseItemDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid item id"})
		return
	}
	if err := h.compareSvc.DeleteFileRowOwnedBy(database.AsSystem(ctx), rowID, currentActorUserID(r)); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// AdminMyTempWarehouseBulkSubmit processes bulk actions on the current user's owned warehouses.
func (h *UIHandler) AdminMyTempWarehouseBulkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", i18n.T(lang, "admin.temp_wh.invalid_data"))
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
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", "لم يتم تحديد أي مستودعات لتنفيذ الإجراء")
		return
	}

	actor, ok := authctx.From(ctx)
	currentUID := currentActorUserID(r)
	var ownerPtr *int64
	// If the user is not staff, restrict bulk operations to their own files
	if !ok || !actor.IsStaff {
		ownerPtr = &currentUID
	}

	var affected int64
	var opErr error

	switch action {
	case "archive":
		reason := i18n.T(lang, "admin.temp_wh.manual_archive_reason")
		affected, opErr = h.compareSvc.BulkArchiveFiles(database.AsSystem(ctx), ids, ownerPtr, reason)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", "لم يتم العثور على مستودعات قابلة للأرشفة أو حدث خطأ أثناء التنفيذ")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", fmt.Sprintf("تم أرشفة %d مستودع بنجاح", affected))
	case "unarchive":
		affected, opErr = h.compareSvc.BulkUnarchiveFiles(database.AsSystem(ctx), ids, ownerPtr)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", "لم يتم العثور على مستودعات قابلة للتفعيل أو حدث خطأ أثناء التنفيذ")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", fmt.Sprintf("تم تفعيل واسترجاع %d مستودع بنجاح", affected))
	case "delete":
		affected, opErr = h.compareSvc.BulkDeleteFiles(database.AsSystem(ctx), ids, ownerPtr)
		if opErr != nil || affected == 0 {
			h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", "لم يتم العثور على المستودعات المحددة لحذفها أو قد تم حذفها مسبقاً")
			return
		}
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "success", fmt.Sprintf("تم حذف %d مستودع وكافة أصنافها نهائياً", affected))
	default:
		h.redirectWithNotice(w, r, tempWarehouseMineBase, "error", "إجراء غير معروف")
	}
}
