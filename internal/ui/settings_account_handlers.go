package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SettingsProfileSubmit saves name, phone, timezone, language, and avatar.
func (h *UIHandler) SettingsProfileSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings#profile", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	name := r.PostFormValue("name")
	nameAr := r.PostFormValue("name_ar")
	if nameAr == "" && name != "" {
		nameAr = name
	}
	nameEn := r.PostFormValue("name_en")
	if nameEn == "" && name != "" {
		nameEn = name
	}

	_, err := h.idSvc.UpdateProfile(ctx, actor.UserID,
		nameAr, nameEn,
		r.PostFormValue("phone"), r.PostFormValue("timezone"),
		r.PostFormValue("lang"),
	)
	if err != nil {
		h.log.WarnContext(ctx, "profile update failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, "/settings#profile", "error", h.safeMessage(err, lang))
		return
	}

	// Handle avatar file upload or removal
	if r.PostFormValue("remove_avatar") == "1" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, "")
	} else if uploadedURL, err := saveUploadedFile(r, "avatar_file", "avatars"); err == nil && uploadedURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, uploadedURL)
	} else if uploadedURL, err := saveUploadedFile(r, "avatar", "avatars"); err == nil && uploadedURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, uploadedURL)
	} else if avatarURL := r.PostFormValue("avatar_url"); avatarURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, avatarURL)
	}

	if h.resolver != nil {
		h.resolver.Invalidate(actor.UserID, actor.OrganizationID)
	}

	h.redirectWithNotice(w, r, "/settings#profile", "success", i18n.T(lang, "settings.profile_updated_success"))
}

// SettingsAddressSubmit adds a saved address.
func (h *UIHandler) SettingsAddressSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/addresses", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/addresses", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	cityID, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	addr := &identity.UserAddress{
		UserID:    actor.UserID,
		Title:     r.PostFormValue("title"),
		Recipient: r.PostFormValue("recipient"),
		Phone:     r.PostFormValue("phone"),
		CityID:    cityID,
		Address:   r.PostFormValue("address"),
		Building:  r.PostFormValue("building"),
		Floor:     r.PostFormValue("floor"),
		Apartment: r.PostFormValue("apartment"),
		IsDefault: r.PostFormValue("is_default") == "1",
	}

	if _, err := h.idSvc.CreateAddress(ctx, addr); err != nil {
		h.redirectWithNotice(w, r, "/settings/addresses", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/settings/addresses", "success", i18n.T(lang, "settings.address_saved_success"))
}

// SettingsAddressDeleteSubmit removes a saved address.
func (h *UIHandler) SettingsAddressDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/addresses", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.idSvc.DeleteAddress(ctx, id, actor.UserID)
		}
	}
	h.redirectWithNotice(w, r, "/settings/addresses", "success", i18n.T(lang, "settings.address_deleted_success"))
}

// SettingsSessionPlanPurchaseSubmit applies a session plan's concurrency limit.
func (h *UIHandler) SettingsSessionPlanPurchaseSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/security", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/security", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	planID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.idSvc.PurchaseSessionPlan(ctx, actor.UserID, planID); err != nil {
		h.redirectWithNotice(w, r, "/settings/security", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/settings/security", "success", i18n.T(lang, "settings.plan_activated_success"))
}

// SettingsSessionRevokeSubmit revokes one of the user's sessions.
func (h *UIHandler) SettingsSessionRevokeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/security", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		_ = h.idSvc.RevokeSession(ctx, r.PostFormValue("token"), actor.UserID)
	}
	h.redirectWithNotice(w, r, "/settings#security", "success", i18n.T(lang, "settings.session_revoked_success"))
}

// SettingsPasswordSubmit updates the user's password from the settings form.
func (h *UIHandler) SettingsPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	curr := r.PostFormValue("current_password")
	newPass := r.PostFormValue("new_password")
	confirmPass := r.PostFormValue("new_password_confirmation")

	if newPass == "" || newPass != confirmPass {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "settings.password_mismatch"))
		return
	}

	if err := h.idSvc.ChangePassword(ctx, actor.UserID, curr, newPass); err != nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/settings#security", "success", i18n.T(lang, "settings.password_changed_success"))
}

// SettingsPreferencesSubmit saves the user's preferences.
func (h *UIHandler) SettingsPreferencesSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/preferences", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/preferences", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	p := &identity.UserPreferences{
		UserID: actor.UserID,
		Theme:  r.PostFormValue("theme"),
		NotificationChannels: map[string]bool{
			"email": r.PostFormValue("ch_email") == "on",
			"sms":   r.PostFormValue("ch_sms") == "on",
			"push":  r.PostFormValue("ch_push") == "on",
		},
		NotificationTopics: map[string]bool{"offers": true, "blog": false, "newsletter": true},
		MarketingConsent:   r.PostFormValue("marketing_consent") == "on",
	}
	if p.Theme == "" {
		p.Theme = "light"
	}
	if err := h.idSvc.UpdatePreferences(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/settings/preferences", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/settings/preferences", "success", i18n.T(lang, "settings.preferences_saved_success"))
}

// SettingsDeleteRequestSubmit receives an account or organization deletion request from a user.
func (h *UIHandler) SettingsDeleteRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	isEmployee := (actor.OrganizationID > 0 && !actor.IsOwner) || actor.Role == "employee" || actor.Role == "org_employee"
	if isEmployee {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "لا يمكن لحسابات الموظفين تقديم طلب لحذف الحساب. يرجى مراجعة إدارة المنشأة.")
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	deleteTarget := strings.TrimSpace(r.PostFormValue("delete_target"))

	if deleteTarget == "organization" && actor.OrganizationID > 0 {
		if !actor.IsOwner {
			h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "عذراً، يحق لمالك المنشأة فقط تقديم طلب حذف المنشأة.")
			return
		}
		if _, err := h.orgSvc.RequestOrganizationDeletion(ctx, actor.OrganizationID, actor.UserID, reason); err != nil {
			h.log.ErrorContext(ctx, "request org deletion from settings", "org_id", actor.OrganizationID, "error", err)
			h.redirectWithNotice(w, r, "/settings?tab=profile", "error", h.errorMessage(r, err))
			return
		}
		h.notifyOrgDeletionRequested(ctx, actor.OrganizationID, reason)
		h.redirectWithNotice(w, r, "/settings?tab=profile", "success", "تم تقديم طلب حذف المنشأة بنجاح وهو قيد مراجعة إدارة المنصة.")
		return
	}

	var orgID *int64
	if actor.OrganizationID > 0 {
		orgID = &actor.OrganizationID
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	if err := h.idSvc.RequestAccountDeletion(ctx, actor.UserID, orgID, reason); err != nil {
		h.log.ErrorContext(ctx, "request account deletion from settings", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", h.errorMessage(r, err))
		return
	}

	h.notifyAccountDeletionRequested(ctx, actor.UserID, reason)
	h.redirectWithNotice(w, r, "/settings?tab=profile", "success", i18n.T(lang, "settings.delete_account_requested_success"))
}

// SettingsAccountDeletionCancelSubmit cancels an active pending account deletion request.
func (h *UIHandler) SettingsAccountDeletionCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	reqID, err := strconv.ParseInt(r.PostFormValue("request_id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "معرف الطلب غير صالح.")
		return
	}
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "الخدمة غير متوفرة حالياً.")
		return
	}
	if err := h.idSvc.CancelAccountDeletion(ctx, actor.UserID, reqID); err != nil {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", h.errorMessage(r, err))
		return
	}
	h.redirectWithNotice(w, r, "/settings?tab=profile", "success", "تم إلغاء طلب حذف الحساب بنجاح.")
}

// SettingsOrgDeletionCancelSubmit cancels an active pending organization deletion request from the settings page.
func (h *UIHandler) SettingsOrgDeletionCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}
	if !actor.IsOwner || actor.OrganizationID <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "عذراً، يحق لمالك المنشأة فقط إلغاء طلب حذف المنشأة.")
		return
	}
	_ = r.ParseForm()
	reqID, err := strconv.ParseInt(r.PostFormValue("request_id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "معرف الطلب غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", "الخدمة غير متوفرة حالياً.")
		return
	}
	if err := h.orgSvc.CancelOrganizationDeletion(ctx, actor.OrganizationID, reqID); err != nil {
		h.redirectWithNotice(w, r, "/settings?tab=profile", "error", h.errorMessage(r, err))
		return
	}
	h.redirectWithNotice(w, r, "/settings?tab=profile", "success", "تم إلغاء طلب حذف المنشأة بنجاح.")
}
