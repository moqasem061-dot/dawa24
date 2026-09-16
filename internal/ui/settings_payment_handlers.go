package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SettingsPaymentMethodsSubmit saves a new payment method.
func (h *UIHandler) SettingsPaymentMethodsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.service_unavailable"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	in, err := readPaymentMethodForm(r)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	pm := &billing.UserPaymentMethod{
		UserID:            actor.UserID,
		Provider:          in.Provider,
		AccountIdentifier: in.Identifier,
		Details:           in.Details,
		IsDefault:         r.PostFormValue("is_default") == "1",
	}

	if err := h.billSvc.AddPaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to add payment method", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.created_success"))
}

// SettingsPaymentMethodEditSubmit updates an existing saved payment method or bank account.
func (h *UIHandler) SettingsPaymentMethodEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.service_unavailable"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	in, err := readPaymentMethodForm(r)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	// UpdatePaymentMethod is scoped by user id, so a foreign method id updates
	// nothing — but a silent no-op reads as success, so the repository answers
	// NotFound and that reaches the caller.
	pm := &billing.UserPaymentMethod{
		ID:                id,
		UserID:            actor.UserID,
		Provider:          in.Provider,
		AccountIdentifier: in.Identifier,
		Details:           in.Details,
		IsDefault:         r.PostFormValue("is_default") == "1",
	}

	if err := h.billSvc.UpdatePaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to update payment method", "error", err, "id", id)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.updated_success"))
}

// SettingsPaymentMethodSetDefaultSubmit marks a payment method as default.
func (h *UIHandler) SettingsPaymentMethodSetDefaultSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.SetDefaultPaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to set default payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.set_default_success"))
}

// SettingsPaymentMethodDeleteSubmit deletes a saved payment method.
func (h *UIHandler) SettingsPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.DeletePaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete payment method", "error", err)
			h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.deleted_success"))
}
