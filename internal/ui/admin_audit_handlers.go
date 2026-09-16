package ui

import (
	"net/http"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAnalyticsPage renders the visitor analytics dashboard.
func (h *UIHandler) AdminAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	analytics := &platformadmin.VisitorAnalytics{
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
		Page:      page,
		PerPage:   limit,
	}
	if h.adminSvc != nil {
		if a, err := h.adminSvc.VisitorAnalyticsWithTotal(ctx, limit, offset); err == nil && a != nil {
			analytics = a
			analytics.Page = page
			analytics.PerPage = limit
		}
	}

	h.renderPage(ctx, w, "render admin analytics", pages.AdminAnalytics(lang, dir, analytics))
}

// AdminAuditPage renders the platform audit trail.
func (h *UIHandler) AdminAuditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	filter := platformadmin.AuditLogFilter{
		Action: action,
		Search: q,
		Limit:  limit,
		Offset: offset,
	}

	var entries []*platformadmin.AuditEntry
	var total int
	if h.adminSvc != nil {
		list, tot, err := h.adminSvc.ListAuditLogWithFilter(ctx, filter)
		if err != nil {
			h.log.WarnContext(ctx, "admin audit: list audit log", "error", err)
		} else {
			for _, e := range list {
				localizeAuditEntry(e, lang)
			}
			entries = list
			total = tot
		}
	}

	values := pages.AdminAuditValues{
		Entries:       entries,
		TotalCount:    total,
		Page:          page,
		PerPage:       limit,
		SelectedActor: q,
		ActionFilter:  action,
		Severity:      severity,
	}

	h.renderPage(ctx, w, "render admin audit", pages.AdminAuditPage(values, lang, dir))
}

func localizeAuditEntry(e *platformadmin.AuditEntry, lang any) {
	if e == nil {
		return
	}
	if e.Title != "" && e.Description != "" && e.Title != e.Action && e.Description != "عملية إدارية مسجلة بالنظام" {
		if e.Severity == "" {
			e.Severity = i18n.T(lang, "admin.audit.severity_info")
		}
		return
	}
	e.Severity = i18n.T(lang, "admin.audit.severity_info")
	switch e.Action {
	case "org.registered":
		e.Module = i18n.T(lang, "admin.audit.mod_orgs")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_org_registered")
		e.Title = i18n.T(lang, "admin.audit.title_org_registered")
		e.Description = i18n.T(lang, "admin.audit.desc_org_registered")
	case "org.approved", "org.status_updated":
		e.Module = i18n.T(lang, "admin.audit.mod_orgs")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_org_status")
		e.Title = i18n.T(lang, "admin.audit.title_org_approved")
		e.Description = i18n.T(lang, "admin.audit.desc_org_approved")
	case "org.rejected":
		e.Module = i18n.T(lang, "admin.audit.mod_orgs")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_org_rejected")
		e.Title = i18n.T(lang, "admin.audit.title_org_rejected")
		e.Description = i18n.T(lang, "admin.audit.desc_org_rejected")
		e.Severity = i18n.T(lang, "admin.audit.severity_critical")
	case "org.suspended":
		e.Module = i18n.T(lang, "admin.audit.mod_orgs")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_org_suspended")
		e.Title = i18n.T(lang, "admin.audit.title_org_suspended")
		e.Description = i18n.T(lang, "admin.audit.desc_org_suspended")
		e.Severity = i18n.T(lang, "admin.audit.severity_warning")
	case "identity.user.registered":
		e.Module = i18n.T(lang, "admin.audit.mod_users")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_user_registered")
		e.Title = i18n.T(lang, "admin.audit.title_user_registered")
		e.Description = i18n.T(lang, "admin.audit.desc_user_registered")
	case "identity.user.status_changed":
		e.Module = i18n.T(lang, "admin.audit.mod_users")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_user_status")
		e.Title = i18n.T(lang, "admin.audit.title_user_status")
		e.Description = i18n.T(lang, "admin.audit.desc_user_status")
		e.Severity = i18n.T(lang, "admin.audit.severity_warning")
	case "identity.user.role_assigned":
		e.Module = i18n.T(lang, "admin.audit.mod_security")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_user_role")
		e.Title = i18n.T(lang, "admin.audit.title_user_role")
		e.Description = i18n.T(lang, "admin.audit.desc_user_role")
		e.Severity = i18n.T(lang, "admin.audit.severity_warning")
	case "identity.user.mfa_reset":
		e.Module = i18n.T(lang, "admin.audit.mod_security")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_user_mfa_reset")
		e.Title = i18n.T(lang, "admin.audit.title_user_mfa_reset")
		e.Description = i18n.T(lang, "admin.audit.desc_user_mfa_reset")
		e.Severity = i18n.T(lang, "admin.audit.severity_critical")
	case "catalog.product.created", "product.created":
		e.Module = i18n.T(lang, "admin.audit.mod_catalog")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_product_created")
		e.Title = i18n.T(lang, "admin.audit.title_product_created")
		e.Description = i18n.T(lang, "admin.audit.desc_product_created")
	case "catalog.product.updated", "product.updated":
		e.Module = i18n.T(lang, "admin.audit.mod_catalog")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_product_updated")
		e.Title = i18n.T(lang, "admin.audit.title_product_updated")
		e.Description = i18n.T(lang, "admin.audit.desc_product_updated")
	case "catalog.product.deleted", "product.deleted":
		e.Module = i18n.T(lang, "admin.audit.mod_catalog")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_product_deleted")
		e.Title = i18n.T(lang, "admin.audit.title_product_deleted")
		e.Description = i18n.T(lang, "admin.audit.desc_product_deleted")
		e.Severity = i18n.T(lang, "admin.audit.severity_critical")
	case "catalog.variant.created", "variant.created":
		e.Module = i18n.T(lang, "admin.audit.mod_offers")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_variant_created")
		e.Title = i18n.T(lang, "admin.audit.title_variant_created")
		e.Description = i18n.T(lang, "admin.audit.desc_variant_created")
	case "order.created":
		e.Module = i18n.T(lang, "admin.audit.mod_orders")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_order_created")
		e.Title = i18n.T(lang, "admin.audit.title_order_created")
		e.Description = i18n.T(lang, "admin.audit.desc_order_created")
	case "order.status_updated", "order.status_changed":
		e.Module = i18n.T(lang, "admin.audit.mod_orders")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_order_status")
		e.Title = i18n.T(lang, "admin.audit.title_order_status")
		e.Description = i18n.T(lang, "admin.audit.desc_order_status")
	case "institutional_work.created":
		e.Module = i18n.T(lang, "admin.audit.mod_institutional")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_inst_created")
		e.Title = i18n.T(lang, "admin.audit.title_inst_created")
		e.Description = i18n.T(lang, "admin.audit.desc_inst_created")
	case "institutional_work.updated":
		e.Module = i18n.T(lang, "admin.audit.mod_institutional")
		e.ActionLabelAr = i18n.T(lang, "admin.audit.action_inst_updated")
		e.Title = i18n.T(lang, "admin.audit.title_inst_updated")
		e.Description = i18n.T(lang, "admin.audit.desc_inst_updated")
	default:
		e.Module = i18n.T(lang, "admin.audit.mod_system")
		e.ActionLabelAr = e.Action
		e.Title = e.Action
		e.Description = i18n.T(lang, "admin.audit.desc_default")
	}

	switch e.EntityType {
	case "organization", "org":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_org")
	case "identity.user", "user":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_user")
	case "catalog.product", "product":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_product")
	case "catalog.variant", "product_variant", "variant":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_variant")
	case "order", "commerce.order":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_order")
	case "branch", "org.branch":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_branch")
	case "institutional_work":
		e.EntityTypeAr = i18n.T(lang, "admin.audit.entity_inst")
	default:
		e.EntityTypeAr = e.EntityType
	}

	if e.ActorName == "" {
		e.ActorName = i18n.T(lang, "admin.audit.default_actor")
	}
	if e.OrganizationName == "" {
		e.OrganizationName = i18n.T(lang, "admin.audit.default_org")
	}
	if e.IPAddress == "" {
		e.IPAddress = "127.0.0.1 (Local)"
	}
	if e.Route == "" {
		e.Route = "/admin/" + e.EntityType
	}
}
