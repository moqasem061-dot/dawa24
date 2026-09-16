package ui

import (
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerMFAPage renders the Multi-Factor Authentication settings page for Pharmacy customers.
func (h *UIHandler) CustomerMFAPage(w http.ResponseWriter, r *http.Request) {
	h.renderMFAPage(w, r, false)
}

// VendorMFAPage renders the Multi-Factor Authentication settings page for Vendors.
func (h *UIHandler) VendorMFAPage(w http.ResponseWriter, r *http.Request) {
	h.renderMFAPage(w, r, true)
}

func (h *UIHandler) renderMFAPage(w http.ResponseWriter, r *http.Request, isVendor bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)

	var status *pages.MFASettingsViewData
	mfaStatus, err := h.idSvc.GetMFAStatus(ctx, actor.UserID)
	if err != nil {
		h.log.WarnContext(ctx, "get mfa status error", "user_id", actor.UserID, "error", err)
	}

	viewData := pages.MFASettingsViewData{
		Status:        mfaStatus,
		IsVendor:      isVendor,
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	_ = status

	h.renderPage(ctx, w, "render mfa settings page", pages.MFASettingsPage(lang, dir, viewData))
}

// CustomerMFASetupSubmit initiates the MFA enrollment flow for Customer.
func (h *UIHandler) CustomerMFASetupSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFASetupSubmit(w, r, false)
}

// VendorMFASetupSubmit initiates the MFA enrollment flow for Vendor.
func (h *UIHandler) VendorMFASetupSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFASetupSubmit(w, r, true)
}

func (h *UIHandler) handleMFASetupSubmit(w http.ResponseWriter, r *http.Request, isVendor bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	dest := "/customer/mfa"
	if isVendor {
		dest = "/vendor/mfa"
	}

	user, err := h.idSvc.GetUserByID(ctx, actor.UserID)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.user_not_found"))
		return
	}

	setupData, err := h.idSvc.SetupMFA(ctx, actor.UserID, user.Email)
	if err != nil {
		h.log.ErrorContext(ctx, "setup mfa error", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.setup_failed"))
		return
	}

	mfaStatus, _ := h.idSvc.GetMFAStatus(ctx, actor.UserID)

	viewData := pages.MFASettingsViewData{
		Status:        mfaStatus,
		SetupData:     setupData,
		IsVendor:      isVendor,
		NoticeType:    "success",
		NoticeMessage: i18n.T(lang, "mfa.scan_qr_instruction"),
	}

	h.renderPage(ctx, w, "render mfa setup page", pages.MFASettingsPage(lang, dir, viewData))
}

// CustomerMFAConfirmSubmit confirms the initial TOTP code and enables MFA.
func (h *UIHandler) CustomerMFAConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFAConfirmSubmit(w, r, false)
}

// VendorMFAConfirmSubmit confirms the initial TOTP code and enables MFA.
func (h *UIHandler) VendorMFAConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFAConfirmSubmit(w, r, true)
}

func (h *UIHandler) handleMFAConfirmSubmit(w http.ResponseWriter, r *http.Request, isVendor bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	dest := "/customer/mfa"
	if isVendor {
		dest = "/vendor/mfa"
	}

	code := strings.TrimSpace(r.PostFormValue("code"))
	if len(code) != 6 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.code_six_digits"))
		return
	}

	actResult, err := h.idSvc.ConfirmEnableMFA(ctx, actor.UserID, code)
	if err != nil {
		h.log.WarnContext(ctx, "confirm mfa failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.code_invalid_or_expired"))
		return
	}

	mfaStatus, _ := h.idSvc.GetMFAStatus(ctx, actor.UserID)

	viewData := pages.MFASettingsViewData{
		Status:        mfaStatus,
		IsVendor:      isVendor,
		NoticeType:    "success",
		NoticeMessage: i18n.T(lang, "mfa.activated_success"),
		RecoveryCodes: actResult.RecoveryCodes,
	}

	h.renderPage(ctx, w, "render mfa activated page", pages.MFASettingsPage(lang, dir, viewData))
}

// CustomerMFADisableSubmit disables MFA after password confirmation.
func (h *UIHandler) CustomerMFADisableSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFADisableSubmit(w, r, false)
}

// VendorMFADisableSubmit disables MFA after password confirmation.
func (h *UIHandler) VendorMFADisableSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFADisableSubmit(w, r, true)
}

func (h *UIHandler) handleMFADisableSubmit(w http.ResponseWriter, r *http.Request, isVendor bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lang := langOf(r)
	dest := "/customer/mfa"
	if isVendor {
		dest = "/vendor/mfa"
	}

	password := r.PostFormValue("password")
	if password == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.password_required_disable"))
		return
	}

	if err := h.idSvc.DisableMFA(ctx, actor.UserID, password, ""); err != nil {
		h.log.WarnContext(ctx, "disable mfa failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.disable_failed"))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "mfa.disabled_success"))
}

// CustomerMFARegenerateRecoveryCodesSubmit generates new recovery codes.
func (h *UIHandler) CustomerMFARegenerateRecoveryCodesSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFARegenerateCodesSubmit(w, r, false)
}

// VendorMFARegenerateRecoveryCodesSubmit generates new recovery codes.
func (h *UIHandler) VendorMFARegenerateRecoveryCodesSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleMFARegenerateCodesSubmit(w, r, true)
}

func (h *UIHandler) handleMFARegenerateCodesSubmit(w http.ResponseWriter, r *http.Request, isVendor bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	dest := "/customer/mfa"
	if isVendor {
		dest = "/vendor/mfa"
	}

	password := r.PostFormValue("password")
	if password == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.password_required_regen"))
		return
	}

	newCodes, err := h.idSvc.RegenerateRecoveryCodes(ctx, actor.UserID, password)
	if err != nil {
		h.log.WarnContext(ctx, "regenerate recovery codes failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "mfa.regen_failed"))
		return
	}

	mfaStatus, _ := h.idSvc.GetMFAStatus(ctx, actor.UserID)

	viewData := pages.MFASettingsViewData{
		Status:        mfaStatus,
		IsVendor:      isVendor,
		NoticeType:    "success",
		NoticeMessage: i18n.T(lang, "mfa.regen_success"),
		RecoveryCodes: newCodes,
	}

	h.renderPage(ctx, w, "render mfa regenerated codes page", pages.MFASettingsPage(lang, dir, viewData))
}
