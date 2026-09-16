package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Smart ordering is an additional way to start a طلب الشراء, not a replacement
// for the purchase-request routes already there (FR-000, FR-058).
//
// RegisterRoutes mounts the smart-order API.
//
// Every route is gated on the same tenant permissions the UI uses
// (internal/ui/customer_routes.go), so a caller who cannot open the smart-order
// screen cannot drive it over JSON either. Reads take pharmacy.smart_order.view;
// anything that starts a run or changes one takes pharmacy.smart_order.run,
// which the registry declares as implying view.
//
// The gates answer JSON 403 rather than the UI's 404 — see ADR 0015. Ownership
// is separately enforced in the service: every lookup is scoped by the caller's
// organization, never by an id taken from the URL.
func RegisterRoutes(r chi.Router, h *Handler, v *Reviewer) {
	r.Group(func(read chi.Router) {
		read.Use(authctx.RequireAPITenantPermission("pharmacy.smart_order.view"))

		read.Get("/api/v1/smart-order", h.History)
		read.Get("/api/v1/smart-order/{id}", h.Get)
		read.Get("/api/v1/smart-order/{id}/results", h.Results)
		read.Get("/api/v1/smart-order/{id}/events", h.Events)
		read.Get("/api/v1/smart-order/{id}/lines/{lineID}/candidates", h.Candidates)
	})

	r.Group(func(write chi.Router) {
		write.Use(authctx.RequireAPITenantPermission("pharmacy.smart_order.run"))

		write.Post("/api/v1/smart-order", h.Start)
		write.Post("/api/v1/smart-order/{id}/mapping", h.ConfirmMapping)
		write.Post("/api/v1/smart-order/{id}/lines/{lineID}/match", v.CorrectMatch)
		write.Post("/api/v1/smart-order/{id}/lines/{lineID}/quantity", v.SetQuantity)
		write.Post("/api/v1/smart-order/{id}/lines/{lineID}/supplier", v.ChooseSupplier)
		write.Post("/api/v1/smart-order/{id}/lines/{lineID}/remove", v.RemoveLine)
		write.Post("/api/v1/smart-order/{id}/finalize", v.Finalize)
	})
}
