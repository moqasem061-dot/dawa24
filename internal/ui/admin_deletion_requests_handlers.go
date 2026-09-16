package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

const deletionRequestsBasePath = "/admin/deletion-requests"

// AdminDeletionRequestsPage renders the unified deletion requests queue for organizations and users.
func (h *UIHandler) AdminDeletionRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "users" {
		tab = "organizations"
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	switch status {
	case "", string(org.OrgDeletionStatusPending), string(org.OrgDeletionStatusApproved),
		string(org.OrgDeletionStatusRejected), string(org.OrgDeletionStatusCancelled):
	default:
		status = ""
	}

	if r.URL.Query().Get("status") == "" && !r.URL.Query().Has("page") {
		status = string(org.OrgDeletionStatusPending)
	}

	page, perPage := 1, 25
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		perPage = n
	}

	var orgRequests []*org.OrganizationDeletionRequest
	var userRequests []*identity.AccountDeletionRequest
	var total int
	var orgPending, orgApproved, orgRejected int
	var userPending, userApproved, userRejected int

	if h.orgSvc != nil {
		_, orgPending, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusPending), 1, 0)
		_, orgApproved, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusApproved), 1, 0)
		_, orgRejected, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusRejected), 1, 0)

		if tab == "organizations" {
			var err error
			orgRequests, total, err = h.orgSvc.ListOrganizationDeletionRequests(ctx, status, perPage, (page-1)*perPage)
			if err != nil {
				h.log.ErrorContext(ctx, "list org deletion requests", "error", err)
				h.renderError(w, r, err)
				return
			}
		}
	}

	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		_, userPending, _ = h.idSvc.AdminListDeletionRequestsWithTotal(sysCtx, "pending", 1, 0)
		_, userApproved, _ = h.idSvc.AdminListDeletionRequestsWithTotal(sysCtx, "approved", 1, 0)
		_, userRejected, _ = h.idSvc.AdminListDeletionRequestsWithTotal(sysCtx, "rejected", 1, 0)

		if tab == "users" {
			var err error
			userRequests, total, err = h.idSvc.AdminListDeletionRequestsWithTotal(sysCtx, status, perPage, (page-1)*perPage)
			if err != nil {
				h.log.ErrorContext(ctx, "list user deletion requests", "error", err)
				h.renderError(w, r, err)
				return
			}
		}
	}

	view := pages.AdminDeletionRequestsView{
		Lang:              lang,
		Tab:               tab,
		Status:            status,
		SearchQuery:       strings.TrimSpace(r.URL.Query().Get("q")),
		Page:              page,
		PerPage:           perPage,
		Total:             total,
		OrgPendingCount:   orgPending,
		OrgApprovedCount:  orgApproved,
		OrgRejectedCount:  orgRejected,
		UserPendingCount:  userPending,
		UserApprovedCount: userApproved,
		UserRejectedCount: userRejected,
		OrgRequests:       orgRequests,
		UserRequests:      userRequests,
		CanDecide:         actorCan(r, "org.organization.update") || actorCan(r, "identity.user.update"),
		NoticeKind:        r.URL.Query().Get("notice_type"),
		Notice:            r.URL.Query().Get("notice_msg"),
	}

	h.renderPage(ctx, w, "unified deletion requests", pages.AdminDeletionRequestsPage(view, lang, dir))
}

// AdminOrgDeletionRequestsLegacyRedirect redirects legacy paths to /admin/deletion-requests?tab=organizations.
func (h *UIHandler) AdminOrgDeletionRequestsLegacyRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !q.Has("tab") {
		q.Set("tab", "organizations")
	}
	target := deletionRequestsBasePath + "?" + q.Encode()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// AdminOrgDeletionRequestsPage satisfies backward-compatibility with old route registration.
func (h *UIHandler) AdminOrgDeletionRequestsPage(w http.ResponseWriter, r *http.Request) {
	h.AdminOrgDeletionRequestsLegacyRedirect(w, r)
}

// AdminOrgDeletionApproveSubmit approves an organization deletion request.
func (h *UIHandler) AdminOrgDeletionApproveSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgDeletion(w, r, true)
}

// AdminOrgDeletionRejectSubmit rejects an organization deletion request.
func (h *UIHandler) AdminOrgDeletionRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgDeletion(w, r, false)
}

func (h *UIHandler) decideOrgDeletion(w http.ResponseWriter, r *http.Request, approve bool) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(deletionRequestsBasePath), http.StatusSeeOther)
		return
	}

	id := parseInt64PathParam(r, "id")
	if id <= 0 {
		h.redirectDeletionNotice(w, r, "organizations", "error", i18n.T(lang, "admin.deletion.invalid_request_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectDeletionNotice(w, r, "organizations", "error", i18n.T(lang, "admin.deletion.invalid_form_data"))
		return
	}
	notes := strings.TrimSpace(r.FormValue("admin_notes"))

	res, err := h.orgSvc.ReviewOrganizationDeletion(ctx, id, actor.UserID, approve, notes)
	if err != nil {
		h.log.ErrorContext(ctx, "decide org deletion request", "request_id", id, "approve", approve, "error", err)
		h.redirectDeletionNotice(w, r, "organizations", "error", h.safeMessage(err, lang))
		return
	}
	if res != nil {
		h.notifyOrgDeletionDecision(ctx, res.OrganizationID, approve, notes)
	}

	if approve {
		h.redirectDeletionNotice(w, r, "organizations", "success", i18n.T(lang, "admin.org.deletion_approved_success"))
	} else {
		h.redirectDeletionNotice(w, r, "organizations", "success", i18n.T(lang, "admin.org.deletion_rejected_success"))
	}
}

// AdminUserDeletionApproveSubmit approves a user account deletion request.
func (h *UIHandler) AdminUserDeletionApproveSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideUserDeletion(w, r, true)
}

// AdminUserDeletionRejectSubmit rejects a user account deletion request.
func (h *UIHandler) AdminUserDeletionRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideUserDeletion(w, r, false)
}

func (h *UIHandler) decideUserDeletion(w http.ResponseWriter, r *http.Request, approve bool) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(deletionRequestsBasePath+"?tab=users"), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectDeletionNotice(w, r, "users", "error", i18n.T(lang, "admin.deletion.invalid_request_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectDeletionNotice(w, r, "users", "error", i18n.T(lang, "admin.deletion.invalid_form_data"))
		return
	}
	notes := strings.TrimSpace(r.FormValue("admin_notes"))
	if approve && notes == "" {
		notes = i18n.T(lang, "admin.users.deletion_approved_reason")
	}

	if h.idSvc != nil {
		if err := h.idSvc.AdminReviewDeletionRequest(ctx, id, actor.UserID, approve, notes); err != nil {
			h.log.ErrorContext(ctx, "decide user deletion request", "request_id", id, "approve", approve, "error", err)
			h.redirectDeletionNotice(w, r, "users", "error", h.safeMessage(err, lang))
			return
		}
		h.notifyAccountDeletionDecision(ctx, id, approve, notes)
	}

	if approve {
		h.redirectDeletionNotice(w, r, "users", "success", i18n.T(lang, "admin.users.deletion_approved_success"))
	} else {
		h.redirectDeletionNotice(w, r, "users", "success", i18n.T(lang, "admin.users.deletion_rejected_success"))
	}
}

func (h *UIHandler) redirectDeletionNotice(w http.ResponseWriter, r *http.Request, tab, kind, msg string) {
	target := fmt.Sprintf("%s?tab=%s&notice_type=%s&notice_msg=%s",
		deletionRequestsBasePath, tab, kind, url.QueryEscape(msg))
	http.Redirect(w, r, target, http.StatusSeeOther)
}
