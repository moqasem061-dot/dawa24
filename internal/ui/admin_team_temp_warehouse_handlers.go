package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// "مستودعات المشرفين تحت إدارتي" — a main moderator's view of their team.
//
// It is the same screen as مستودعاتي المرفوعة with a different scope, and the
// scope is the entire feature. Three rules, and each is enforced here rather
// than relied upon from the sidebar:
//
//  1. The listing is filtered to the ids of moderators reporting to the caller.
//     Not "their organisation", not "everything they can see" — the exact set
//     that identity.ModeratorSubordinateIDs returns.
//  2. A main moderator with nobody under them sees an empty list. The filter
//     carries a non-nil, empty OwnerIn for exactly this, because an empty scope
//     that degrades to "no filter" is how a permission leak looks in practice.
//  3. Every per-file action re-derives the set and refuses a file outside it.
//     A permission says a moderator may manage their team's warehouses; only
//     this says whose those are, so the check cannot live in the route gate.
//
// The caller's own uploads are deliberately NOT included: they have their own
// screen, and "the warehouses of the people who report to me" must mean that.

const tempWarehouseTeamBase = "/admin/team/temparte-warehouses"

// AdminTeamTempWarehousesPage lists the uploads of the caller's subordinates.
func (h *UIHandler) AdminTeamTempWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	team := h.teamModeratorIDs(ctx)
	filter := compare.AdminTempWarehouseFilter{
		Search:    strings.TrimSpace(r.URL.Query().Get("q")),
		OwnerIn:   team,
		SortBy:    strings.TrimSpace(r.URL.Query().Get("sort")),
		SortOrder: strings.TrimSpace(r.URL.Query().Get("order")),
	}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		st := compare.CompareFileStatus(s)
		filter.Status = &st
	}
	// A team member may be selected from the dropdown, but only one who is
	// actually on the team: an uploader id from the query string is a claim,
	// not an authorisation.
	if u := strings.TrimSpace(r.URL.Query().Get("uploader")); u != "" {
		if id, err := strconv.ParseInt(u, 10, 64); err == nil && containsID(team, id) {
			filter.UploaderID = &id
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	data := h.buildTempWarehousesData(ctx, filter, true, page, limit)
	data.Base = tempWarehouseTeamBase
	data.PageURL = tempWarehouseTeamBase
	if data.ActiveRun != nil {
		data.ActiveRun.BaseURL = tempWarehouseTeamBase
	}
	data.TeamView = true
	data.TeamSize = len(team)
	data.NoticeMsg = strings.TrimSpace(r.URL.Query().Get("notice"))
	data.NoticeType = strings.TrimSpace(r.URL.Query().Get("notice_type"))
	if data.NoticeMsg == "" {
		data.NoticeMsg = strings.TrimSpace(r.URL.Query().Get("msg"))
	}
	if h.idSvc != nil {
		if mods, err := h.idSvc.ListModerators(database.AsSystem(ctx)); err == nil {
			for _, m := range mods {
				if m != nil && containsID(team, m.UserID) {
					data.TeamMembers = append(data.TeamMembers, pages.TeamModerator{
						UserID: m.UserID,
						Name:   m.Name.Get("ar"),
						Email:  m.Email,
					})
				}
			}
		}
	}

	h.renderPage(ctx, w, "render team temp warehouses", pages.AdminTempWarehousesPage(data, lang, dir))
}

// teamModeratorIDs is the caller's direct reports.
//
// It returns a non-nil empty slice when the caller leads nobody, so the filter
// scopes to nothing rather than to everything. That distinction is the whole
// safety property of this screen.
func (h *UIHandler) teamModeratorIDs(ctx context.Context) []int64 {
	if h.idSvc == nil {
		return []int64{}
	}
	uid := int64(0)
	if a, ok := authctx.From(ctx); ok {
		uid = a.UserID
	}
	if uid <= 0 {
		return []int64{}
	}
	ids, err := h.idSvc.ModeratorSubordinateIDs(database.AsSystem(ctx), uid)
	if err != nil {
		h.log.ErrorContext(ctx, "list moderator subordinates", "error", err, "user_id", uid)
		return []int64{}
	}
	if ids == nil {
		return []int64{}
	}
	return ids
}

// teamTempWarehouseOwned parses {id} and returns the file when it belongs to a
// moderator reporting to the caller. Otherwise it answers 403 and returns false.
func (h *UIHandler) teamTempWarehouseOwned(w http.ResponseWriter, r *http.Request) (*compare.CompareFile, bool) {
	ctx := r.Context()
	fileID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || fileID <= 0 {
		h.teamTempWarehouseReject(w, r, http.StatusBadRequest, "معرّف المستودع غير صالح.")
		return nil, false
	}
	f, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || f == nil {
		h.teamTempWarehouseReject(w, r, http.StatusNotFound, "المستودع غير موجود.")
		return nil, false
	}
	if !containsID(h.teamModeratorIDs(ctx), f.UserID) {
		h.log.WarnContext(ctx, "team temp warehouse access outside hierarchy",
			"file", fileID, "owner", f.UserID, "actor", currentActorUserID(r))
		h.teamTempWarehouseReject(w, r, http.StatusForbidden,
			"هذا المستودع لا يتبع أحد المشرفين تحت إدارتك.")
		return nil, false
	}
	return f, true
}

func (h *UIHandler) teamTempWarehouseReject(w http.ResponseWriter, r *http.Request, code int, msg string) {
	if isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
		return
	}
	h.redirectWithNotice(w, r, tempWarehouseTeamBase, "error", msg)
}

// containsID reports membership in a small id set.
func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// AdminTeamTempWarehouseItemsJSON returns one team warehouse's rows.
func (h *UIHandler) AdminTeamTempWarehouseItemsJSON(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseItemsJSON(w, r)
}

// AdminTeamTempWarehouseMappingJSON returns one team warehouse's column mapping.
func (h *UIHandler) AdminTeamTempWarehouseMappingJSON(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseMappingJSON(w, r)
}

// AdminTeamTempWarehouseExportXLSX exports one team warehouse.
func (h *UIHandler) AdminTeamTempWarehouseExportXLSX(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseExportXLSX(w, r)
}

// AdminTeamTempWarehouseToggleArchiveSubmit archives or restores a team warehouse.
func (h *UIHandler) AdminTeamTempWarehouseToggleArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseToggleArchiveSubmit(w, r)
}

// AdminTeamTempWarehouseItemDeleteSubmit deletes one row of a team warehouse.
func (h *UIHandler) AdminTeamTempWarehouseItemDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseItemDeleteSubmit(w, r)
}

// AdminTeamTempWarehouseMappingSubmit re-maps a team warehouse's columns.
func (h *UIHandler) AdminTeamTempWarehouseMappingSubmit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseMappingSubmit(w, r)
}

// AdminTeamTempWarehouseDeleteSubmit deletes a team warehouse.
func (h *UIHandler) AdminTeamTempWarehouseDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.teamTempWarehouseOwned(w, r); !ok {
		return
	}
	h.AdminTempWarehouseDeleteSubmit(w, r)
}

// AdminTeamTempWarehouseBulkSubmit handles bulk actions on team warehouses.
func (h *UIHandler) AdminTeamTempWarehouseBulkSubmit(w http.ResponseWriter, r *http.Request) {
	h.AdminTempWarehouseBulkSubmit(w, r)
}
