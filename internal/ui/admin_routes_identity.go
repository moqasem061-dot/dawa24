package ui

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminIdentityRoutes(r chi.Router) {
	// Users Directory & Classifications
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.user.view"))
		g.Get("/admin/users", h.AdminUsersPage)
		g.Get("/admin/users/{id}", h.AdminUserDetailPage)
		g.Get("/admin/users/{id}/info", h.AdminUserDetailPage)
		g.Get("/admin/users/{id}/edit", h.AdminUserDetailPage)

		// 301 Redirects for duplicate user screens (Task B.1)
		g.Get("/admin/full-user", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-user/new-clients", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=new", http.StatusMovedPermanently)
		})
		g.Get("/admin/customer-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=customer", http.StatusMovedPermanently)
		})
		g.Get("/admin/customer-list/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/vendor-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=vendor", http.StatusMovedPermanently)
		})
		g.Get("/admin/vendor-list/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/admin-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=staff", http.StatusMovedPermanently)
		})
		g.Get("/admin/admins", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=staff", http.StatusMovedPermanently)
		})
		g.Get("/admin/admins/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/admins/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s/edit", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})

		g.Get("/admin/user-address", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/user-address/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/user-organization", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/want-delete", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/deletion-requests?tab=users", http.StatusMovedPermanently)
		})
		g.Get("/admin/want-delete/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/deletion-requests?tab=users", http.StatusMovedPermanently)
		})

	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.activity.view"))
		g.Get("/admin/employee-activities", h.AdminEmployeeActivitiesPage)
		g.Get("/admin/employee-activities/export", h.AdminEmployeeActivitiesExport)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.user.update"))
		g.Post("/admin/users/{id}/edit", h.AdminUserEditSubmit)
		g.Post("/admin/users/{id}/password", h.AdminUserPasswordSubmit)
		g.Post("/admin/users/{id}/suspend", h.AdminUserSuspendSubmit)
		g.Post("/admin/users/{id}/reactivate", h.AdminUserReactivateSubmit)
		g.Post("/admin/users/{id}/reset-mfa", h.AdminUserResetMFASubmit)
		g.Post("/admin/deletion-requests/users/{id}/approve", h.AdminUserDeletionApproveSubmit)
		g.Post("/admin/deletion-requests/users/{id}/reject", h.AdminUserDeletionRejectSubmit)
		g.Post("/admin/users/deletion/{id}/approve", h.AdminUserDeletionApproveSubmit)
		g.Post("/admin/users/deletion/{id}/reject", h.AdminUserDeletionRejectSubmit)
	})

	// Roles & permissions. Reading the role list is not the same right as
	// editing a role, and editing a role is not the same right as assigning
	// one to a user: whoever can assign roles can grant themselves anything
	// any role holds, so it is granted separately and withheld from the
	// starter "Administrator" role.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.admin_role.view"))
		g.Get("/admin/roles", h.AdminRolesPage)
		g.Get("/admin/roles/{key}", h.AdminRoleDetailPage)
		g.Get("/admin/admin-roles", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/roles", http.StatusMovedPermanently)
		})
		g.Get("/admin/admin-roles/{key}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/roles/"+chi.URLParam(r, "key"), http.StatusMovedPermanently)
		})
		g.Get("/admin/admin-permissions", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/roles", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.admin_role.update"))
		g.Post("/admin/roles", h.AdminRoleCreateSubmit)
		g.Post("/admin/roles/{key}", h.AdminRoleUpdateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.admin_role.delete"))
		g.Post("/admin/roles/{key}/delete", h.AdminRoleDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.admin_role.assign"))
		g.Post("/admin/users/{id}/role", h.AdminUserRoleAssignSubmit)
		g.Post("/admin/users/staff", h.AdminStaffCreateSubmit)
	})

	// The moderator hierarchy. Its own permission: deciding who a moderator
	// reports to decides who may read their uploaded warehouses, which is an
	// access-control change and not a profile edit.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.moderator.assign"))
		g.Post("/admin/users/{id}/moderator-parent", h.AdminModeratorParentSubmit)
	})
}
