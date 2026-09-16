package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminOrgRoutes(r chi.Router) {
	// Organizations & Approvals
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.view"))
		g.Get("/admin/organizations", h.AdminOrganizationsPage)
		g.Get("/admin/organizations/{id}", h.AdminOrganizationDetailPage)
		g.Get("/admin/organizations/{id}/info", h.AdminOrganizationInfoPage)
		g.Get("/admin/organizations/{id}/users", h.AdminOrganizationUsersPage)
		g.Get("/admin/organizations/{id}/branches", h.AdminOrganizationBranchesPage)
		g.Get("/admin/vendors", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/organizations?type=vendor", http.StatusMovedPermanently)
		})
		g.Get("/admin/suppliers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/organizations?type=vendor", http.StatusMovedPermanently)
		})
		g.Get("/admin/organizations/deletion-requests", h.AdminOrgDeletionRequestsLegacyRedirect)
		g.Get("/admin/deletion-requests", h.AdminDeletionRequestsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.update"))
		g.Post("/admin/organizations/{id}/edit", h.AdminOrgEditSubmit)
		g.Post("/admin/organizations/{id}/suspend", h.AdminOrgSuspendSubmit)
		g.Post("/admin/organizations/{id}/extra-devices", h.AdminOrgExtraDevicesSubmit)
		g.Post("/admin/deletion-requests/organizations/{id}/approve", h.AdminOrgDeletionApproveSubmit)
		g.Post("/admin/deletion-requests/organizations/{id}/reject", h.AdminOrgDeletionRejectSubmit)
		g.Post("/admin/organizations/deletion-requests/{id}/approve", h.AdminOrgDeletionApproveSubmit)
		g.Post("/admin/organizations/deletion-requests/{id}/reject", h.AdminOrgDeletionRejectSubmit)
	})

	// Approving an organization admits a company to the marketplace. It is a
	// separate right from editing one, because the reviewer who checks a
	// pharmacy licence is not the operator who corrects a typo in its address.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.approval.view"))
		g.Get("/admin/approvals", h.AdminApprovalsPage)
		g.Get("/admin/approvals/organizations/{id}/details", h.AdminOrgRegistrationFragment)
		g.Get("/admin/organizations/change-requests", h.AdminOrgChangesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.approval.decide"))
		g.Post("/admin/organizations/{id}/approve", h.AdminOrgApproveSubmit)
		g.Post("/admin/organizations/{id}/reject", h.AdminOrgRejectSubmit)
		g.Post("/admin/approvals/{id}/approve", h.AdminApproveOrgSubmit)
		g.Post("/admin/approvals/{id}/reject", h.AdminRejectOrgSubmit)
		g.Post("/admin/approvals/{id}/review", h.AdminOrgReviewSubmit)
		g.Post("/admin/approvals/job-seekers/{id}/approve", h.AdminApproveJobSeekerSubmit)
		g.Post("/admin/approvals/job-seekers/{id}/reject", h.AdminRejectJobSeekerSubmit)
		g.Post("/admin/organizations/change-requests/{id}/approve", h.AdminOrgChangeApproveSubmit)
		g.Post("/admin/organizations/change-requests/{id}/reject", h.AdminOrgChangeRejectSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.document.view"))
		g.Get("/admin/documents/{id}/view", h.DocumentViewHandler)
		g.Get("/admin/documents/{id}/download", h.DocumentDownloadHandler)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.document.update"))
		g.Post("/admin/document-requests", h.AdminCreateDocumentRequestSubmit)
		g.Post("/admin/document-requests/{id}/cancel", h.AdminCancelDocumentRequestSubmit)
		g.Post("/admin/documents/{id}/verify", h.AdminVerifyUploadedDocSubmit)
	})

	// Branches Oversight & Management
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.branch.view"))
		g.Get("/admin/branches", h.AdminBranchesPage)
		g.Get("/admin/branches/{id}", h.AdminBranchDetailPage)
		g.Get("/admin/branches/{id}/products", h.AdminBranchProductsPage)
		g.Get("/admin/branches/{id}/users", h.AdminBranchUsersPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.branch.update"))
		g.Post("/admin/branches/new", h.AdminBranchNewSubmit)
		g.Post("/admin/branches/{id}/edit", h.AdminBranchEditSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.branch.delete"))
		g.Post("/admin/branches/{id}/delete", h.AdminBranchDeleteSubmit)
	})

	// Weekly Coverages Oversight & Management
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.coverage.view"))
		g.Get("/admin/weekly-coverages", h.AdminWeeklyCoveragesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.coverage.update"))
		g.Post("/admin/weekly-coverages/new", h.AdminWeeklyCoverageCreateSubmit)
		g.Post("/admin/weekly-coverages/{id}/toggle", h.AdminWeeklyCoverageToggleSubmit)
		g.Post("/admin/weekly-coverages/{id}/delete", h.AdminWeeklyCoverageDeleteSubmit)
	})

	// Institutional Works
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.view"))
		g.Get("/admin/institutional", h.AdminInstitutionalPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.update"))
		g.Post("/admin/institutional", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/new", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/{id}", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/edit", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/status", h.AdminInstitutionalStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.delete"))
		g.Post("/admin/institutional/{id}/delete", h.AdminInstitutionalDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.review.view"))
		g.Get("/admin/reviews", h.AdminReviewsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.review.update"))
		g.Post("/admin/reviews/{id}/status", h.AdminReviewStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.review.delete"))
		g.Post("/admin/reviews/{id}/delete", h.AdminReviewDeleteSubmit)
	})
}
