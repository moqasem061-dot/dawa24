package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The administrator's side of the organization-profile workflow.

const orgChangesPath = "/admin/organizations/change-requests"

// AdminOrgChangesPage lists profile change requests awaiting a decision.
func (h *UIHandler) AdminOrgChangesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	switch status {
	case "", string(org.ChangePending), string(org.ChangeApproved),
		string(org.ChangeRejected), string(org.ChangeWithdrawn):
	default:
		status = ""
	}
	// The queue is a backlog to work to zero, so it opens on what still needs
	// a decision rather than on every request ever made.
	if r.URL.Query().Get("status") == "" && !r.URL.Query().Has("page") && !r.URL.Query().Has("q") {
		status = string(org.ChangePending)
	}

	var orgID int64
	if id, err := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64); err == nil && id > 0 {
		orgID = id
	}

	section := strings.TrimSpace(r.URL.Query().Get("section"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))

	var dateFrom, dateTo *time.Time
	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			dateFrom = &t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			endOfDay := t.Add(24*time.Hour - time.Nanosecond)
			dateTo = &endOfDay
		}
	}

	page, perPage := 1, 25
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		perPage = n
	}

	filter := org.ProfileChangeFilter{
		Status:         status,
		OrganizationID: orgID,
		Section:        section,
		Search:         search,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
	}

	var orgs []*org.Organization
	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		orgs, _ = h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
	}

	counts, _ := h.orgSvc.ProfileChangeRequestCounts(ctx)
	requests, total, err := h.orgSvc.ListProfileChangeRequestsWithFilter(ctx, filter, perPage, (page-1)*perPage)
	if err != nil {
		h.log.ErrorContext(ctx, "list profile change requests", "error", err)
		h.renderError(w, r, err)
		return
	}

	view := pages.AdminOrgChangesView{
		Lang:          lang,
		Rows:          h.decorateOrgChanges(r, requests),
		Status:        status,
		Section:       section,
		Search:        search,
		DateFrom:      fromStr,
		DateTo:        toStr,
		Counts:        counts,
		Total:         total,
		Page:          page,
		PerPage:       perPage,
		Organizations: orgs,
		SelectedOrgID: orgID,
		CanDecide:     actorCan(r, "org.approval.decide"),
		NoticeKind:    r.URL.Query().Get("notice_type"),
		Notice:        r.URL.Query().Get("notice_msg"),
	}
	h.renderPage(ctx, w, "organization change requests", pages.AdminOrgChangesPage(view, lang, dir))
}

// decorateOrgChanges names each request's organization and users.
func (h *UIHandler) decorateOrgChanges(
	r *http.Request, requests []*org.ProfileChangeRequest,
) []pages.AdminOrgChangeRow {
	ctx := r.Context()
	lang := langOf(r)
	names := map[int64]string{}
	rows := make([]pages.AdminOrgChangeRow, 0, len(requests))

	for _, req := range requests {
		name := req.OrganizationName
		if name == "" {
			var ok bool
			name, ok = names[req.OrganizationID]
			if !ok {
				name = strconv.FormatInt(req.OrganizationID, 10)
				if o, err := h.orgSvc.GetOrganization(ctx, req.OrganizationID); err == nil && o != nil {
					if display := o.TradeName.Get(i18n.ParseLang(lang)); display != "" {
						name = display
					} else if o.LegalName != "" {
						name = o.LegalName
					}
				}
				names[req.OrganizationID] = name
			}
		}
		rows = append(rows, pages.AdminOrgChangeRow{
			Request:       req,
			OrgName:       name,
			RequesterName: req.RequesterName,
			ReviewerName:  req.ReviewerName,
		})
	}
	return rows
}

// AdminOrgChangeApproveSubmit applies a pending request.
func (h *UIHandler) AdminOrgChangeApproveSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgChange(w, r, true)
}

// AdminOrgChangeRejectSubmit refuses one, with a reason the company will read.
func (h *UIHandler) AdminOrgChangeRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgChange(w, r, false)
}

func (h *UIHandler) decideOrgChange(w http.ResponseWriter, r *http.Request, approve bool) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(orgChangesPath), http.StatusSeeOther)
		return
	}

	id := parseInt64PathParam(r, "id")
	if id <= 0 {
		h.redirectWithNotice(w, r, orgChangesPath, "error", i18n.T(lang, "admin.org_changes.invalid_id"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, orgChangesPath, "error", i18n.T(lang, "errors.400_bad_request"))
		return
	}
	notes := strings.TrimSpace(r.FormValue("notes"))

	decided, err := h.orgSvc.DecideProfileChangeRequest(ctx, id, actor.UserID, approve, notes)
	if err != nil {
		h.log.ErrorContext(ctx, "decide profile change request",
			"request_id", id, "approve", approve, "error", err)
		h.redirectWithNotice(w, r, orgChangesPath, "error", h.errorMessage(r, err))
		return
	}
	if decided != nil {
		h.notifyProfileChangeDecision(ctx, decided.OrganizationID, string(decided.Section), approve, notes)
	}

	msg := i18n.T(lang, "admin.org_changes.approved_success")
	if !approve {
		msg = i18n.T(lang, "admin.org_changes.rejected_success")
	}
	h.redirectWithNotice(w, r, orgChangesPath, "success", msg)
}

// actorCan answers a permission question for a request whose actor may be absent.
func actorCan(r *http.Request, permission string) bool {
	actor, ok := authctx.From(r.Context())
	return ok && actor.Can(permission)
}
