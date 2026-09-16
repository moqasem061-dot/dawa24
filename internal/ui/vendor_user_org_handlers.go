package ui

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorUserSearchJSON provides live AJAX user search for vendors to link customers/pharmacies.
func (h *UIHandler) VendorUserSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsVendor() && !actor.IsStaff) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sysCtx := database.AsSystem(ctx)

	type UserSearchResult struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
		Role  string `json:"role"`
	}

	var results []UserSearchResult
	if h.idSvc != nil {
		users, err := h.idSvc.SearchUsers(sysCtx, q, "", 20)
		if err == nil {
			for _, u := range users {
				if u == nil {
					continue
				}
				name := u.Name.Get("ar")
				if name == "" {
					name = u.Name.Get("en")
				}
				if name == "" {
					name = u.Email
				}
				results = append(results, UserSearchResult{
					ID:    u.ID,
					Name:  name,
					Email: u.Email,
					Phone: u.Phone,
					Role:  u.Role,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

// VendorUserOrganizationsPage renders all customer organization number links for the logged in vendor.
func (h *UIHandler) VendorUserOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	data := &pages.VendorUserOrgData{
		ActiveTab:  statusFilter,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice_msg"),
		Page:       page,
		PerPage:    limit,
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		allLinks, _ := h.orgSvc.ListUserOrganizationsByVendor(sysCtx, orgID, "")
		data.TotalCount = len(allLinks)
		for _, link := range allLinks {
			if link == nil {
				continue
			}
			switch link.Status {
			case org.UserOrgStatusPending:
				data.PendingCount++
			case org.UserOrgStatusApproved:
				data.ApprovedCount++
			case org.UserOrgStatusRejected:
				data.RejectedCount++
			}
		}

		filteredLinks, total, err := h.orgSvc.ListUserOrganizationsByVendorWithTotal(sysCtx, orgID, statusFilter, limit, offset)
		if err == nil {
			data.UserOrgs = filteredLinks
			data.FilteredTotal = total
		}
	}

	h.renderPage(ctx, w, "render vendor user organizations", pages.VendorUserOrganizationsPage(lang, dir, data))
}

// VendorUserOrganizationCreateSubmit creates and immediately approves a customer link.
func (h *UIHandler) VendorUserOrganizationCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("user_id")), 10, 64)
	if err != nil || targetUserID <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.select_valid_user")), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	if orgNumber == "" {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.org_number_required")), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		_, err := h.orgSvc.CreateUserOrgLink(sysCtx, targetUserID, nil, orgID, orgNumber, org.UserOrgStatusApproved)
		if err != nil {
			h.log.ErrorContext(ctx, "vendor create user org link failed", "error", err)
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(h.safeMessage(err, lang)), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.linked_approved_success")), http.StatusSeeOther)
}

// VendorUserOrganizationApproveSubmit approves a pending pharmacy connection.
func (h *UIHandler) VendorUserOrganizationApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.invalid_id")), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.ApproveUserOrgLink(sysCtx, id); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(h.safeMessage(err, lang)), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.approved_success")), http.StatusSeeOther)
}

// VendorUserOrganizationRejectSubmit rejects a pharmacy connection request.
func (h *UIHandler) VendorUserOrganizationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.invalid_id")), http.StatusSeeOther)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.RejectUserOrgLink(sysCtx, id, notes); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(h.safeMessage(err, lang)), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.rejected_success")), http.StatusSeeOther)
}

// VendorUserOrganizationUpdateSubmit updates the organization number for a user.
func (h *UIHandler) VendorUserOrganizationUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.invalid_id")), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.UpdateUserOrgLink(sysCtx, id, orgNumber, notes); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(h.safeMessage(err, lang)), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.updated_success")), http.StatusSeeOther)
}

// VendorUserOrganizationDeleteSubmit removes a user link.
func (h *UIHandler) VendorUserOrganizationDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.invalid_id")), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.DeleteUserOrgLink(sysCtx, id); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(h.safeMessage(err, lang)), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape(i18n.T(lang, "vendor.user_org.deleted_success")), http.StatusSeeOther)
}
