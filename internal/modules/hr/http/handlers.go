package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes HR HTTP endpoints.
type Handler struct {
	service *hr.Service
	log     *slog.Logger
}

// NewHandler creates an HR HTTP handler.
func NewHandler(service *hr.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers HR routes on a Chi router.
//
// Creating an employee record and writing working hours are team-management
// actions, gated on the same keys the team screens use rather than on mere
// membership of an approved organisation.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/hr/employees/{id}", h.GetEmployee)
	r.Get("/api/v1/hr/employees", h.ListEmployees)
	r.Get("/api/v1/hr/work-times", h.ListWorkTimes)

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission(
			"vendor.team.create", "pharmacy.team.create", "hr.job.manage", "hr.admin"))
		g.Post("/api/v1/hr/employees", h.CreateEmployee)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission(
			"vendor.team.update", "pharmacy.team.update", "hr.job.manage", "hr.admin"))
		g.Post("/api/v1/hr/work-times", h.SaveWorkTimes)
	})

	h.RegisterAdminRoutes(r)
}

// CreateEmployee handles onboarding a staff member.
func (h *Handler) CreateEmployee(w http.ResponseWriter, r *http.Request) {
	var e hr.Employee
	if err := httpx.DecodeJSON(w, r, &e); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateEmployee(r.Context(), &e)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// GetEmployee retrieves an employee profile.
func (h *Handler) GetEmployee(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid employee ID", nil))
		return
	}

	emp, err := h.service.GetEmployee(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, emp)
}

// ListEmployees returns employees.
func (h *Handler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	employees, err := h.service.ListEmployees(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"employees": employees})
}

// SaveWorkTimes updates operating hours.
func (h *Handler) SaveWorkTimes(w http.ResponseWriter, r *http.Request) {
	var times []*hr.WorkTime
	if err := httpx.DecodeJSON(w, r, &times); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.SaveWorkTimes(r.Context(), times); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// ListWorkTimes returns operating hours.
func (h *Handler) ListWorkTimes(w http.ResponseWriter, r *http.Request) {
	times, err := h.service.ListWorkTimes(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"work_times": times})
}
