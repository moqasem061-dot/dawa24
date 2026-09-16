package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// بيانات المنشأة, for suppliers and pharmacies alike.
//
// One pair of handlers serves /vendor/organization and /customer/organization.
// The two audiences differ only in the URL they post back to and the public
// page they link out to; the sections, the validation and the review policy are
// the organization's, not the audience's. Writing it twice is how the two would
// drift.

// organizationProfileBase returns the caller's own action base, so a form
// posts back to the tier the caller was admitted through.
func organizationProfileBase(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/vendor/") {
		return "/vendor/organization"
	}
	return "/customer/organization"
}

// OrganizationProfilePage renders every section with its stored values and any
// open change request.
func (h *UIHandler) OrganizationProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := organizationProfileBase(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(base), http.StatusSeeOther)
		return
	}
	orgID := actor.OrganizationID

	stored := map[org.ProfileSection]org.ProfileFields{}
	for _, section := range org.ProfileSections() {
		fields, err := h.orgSvc.ReadProfileSection(ctx, orgID, section)
		if err != nil {
			h.log.ErrorContext(ctx, "read organization profile section",
				"org_id", orgID, "section", section, "error", err)
			h.renderError(w, r, err)
			return
		}
		stored[section] = fields
	}

	pending, err := h.orgSvc.PendingProfileChanges(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "read pending profile changes", "org_id", orgID, "error", err)
		h.renderError(w, r, err)
		return
	}

	orgModel, _ := h.orgSvc.GetOrganization(ctx, orgID)
	orgLegalName := ""
	if orgModel != nil {
		if display := orgModel.TradeName.Get(i18n.ParseLang(lang)); display != "" {
			orgLegalName = display
		} else if orgModel.LegalName != "" {
			orgLegalName = orgModel.LegalName
		}
	}

	pendingDeletion, _ := h.orgSvc.GetPendingOrganizationDeletion(ctx, orgID)
	isOwner := actor.IsOwner || (orgModel != nil && orgModel.OwnerID == actor.UserID)

	view := pages.OrganizationProfileView{
		Lang:            lang,
		Title:           i18n.T(lang, "org.profile.title"),
		OrgID:           orgID,
		OrgLegalName:    orgLegalName,
		IsVendor:        strings.HasPrefix(base, "/vendor"),
		ActionBase:      base,
		CanUpdate:       actor.Can(organizationUpdatePerm(base)),
		IsOwner:         isOwner,
		PendingDeletion: pendingDeletion,
		Sections:        pages.BuildOrganizationSections(lang, stored, pending),
		NoticeKind:      r.URL.Query().Get("notice_type"),
		Notice:          r.URL.Query().Get("notice_msg"),
	}
	if view.IsVendor {
		view.PublicURL = fmt.Sprintf("/suppliers/%d", orgID)
	}

	h.renderPage(ctx, w, "organization profile", pages.OrganizationProfilePage(view, lang, dir))
}

func organizationUpdatePerm(base string) string {
	if strings.HasPrefix(base, "/vendor") {
		return "vendor.organization.update"
	}
	return "pharmacy.organization.update"
}

// OrganizationProfileSectionSubmit saves exactly one section.
//
// The old page posted every field of every section at once, so saving a phone
// number rewrote the trade name too. A section that the person did not open is
// now simply not in the request body, and therefore not written.
func (h *UIHandler) OrganizationProfileSectionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	base := organizationProfileBase(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(base), http.StatusSeeOther)
		return
	}

	section := org.ProfileSection(chi.URLParam(r, "section"))
	if !section.Valid() {
		http.NotFound(w, r)
		return
	}

	fields, err := h.readProfileSectionForm(r, section)
	if err != nil {
		organizationProfileNotice(w, r, base, section, "error", h.safeMessage(err, langOf(r)))
		return
	}

	res, err := h.orgSvc.SaveProfileSection(ctx, org.SaveProfileSection{
		OrganizationID: actor.OrganizationID,
		UserID:         actor.UserID,
		Section:        section,
		Fields:         fields,
	})
	if err != nil {
		h.log.ErrorContext(ctx, "save organization profile section",
			"org_id", actor.OrganizationID, "section", section, "error", err)
		organizationProfileNotice(w, r, base, section, "error", h.errorMessage(r, err))
		return
	}

	msg := i18n.T(lang, "org.profile.saved")
	if res.Request != nil {
		msg = i18n.T(lang, "vendor.org.request_submitted_success")
		h.notifyProfileChangeRequested(ctx, actor.OrganizationID, string(section))
	}
	organizationProfileNotice(w, r, base, section, "success", msg)
}

// OrganizationProfileWithdrawSubmit takes back a company's own pending request.
func (h *UIHandler) OrganizationProfileWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	base := organizationProfileBase(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(base), http.StatusSeeOther)
		return
	}

	id := parseInt64PathParam(r, "id")
	if id <= 0 {
		http.NotFound(w, r)
		return
	}

	// Scoped by the caller's organization, so the id in the URL cannot reach
	// another company's request.
	if err := h.orgSvc.WithdrawProfileChangeRequest(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "withdraw profile change request",
			"org_id", actor.OrganizationID, "request_id", id, "error", err)
		organizationProfileNotice(w, r, base, "", "error", h.errorMessage(r, err))
		return
	}
	organizationProfileNotice(w, r, base, "", "success", i18n.T(lang, "org.profile.withdrawn"))
}

// organizationProfileNotice returns to the section the person was working in,
// rather than to the top of a five-section page.
func organizationProfileNotice(
	w http.ResponseWriter, r *http.Request, base string, section org.ProfileSection, kind, msg string,
) {
	target := fmt.Sprintf("%s?notice_type=%s&notice_msg=%s", base, kind, url.QueryEscape(msg))
	if section != "" {
		target += "#section-" + string(section)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// OrganizationDeletionRequestSubmit handles the owner's request to delete the organization.
func (h *UIHandler) OrganizationDeletionRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := organizationProfileBase(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(base), http.StatusSeeOther)
		return
	}

	orgModel, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
	if err != nil {
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", "تعذر العثور على بيانات المنشأة.")
		return
	}

	isOwner := actor.IsOwner || (orgModel != nil && orgModel.OwnerID == actor.UserID)
	if !isOwner {
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", "عذراً، يحق لمالك المنشأة فقط تقديم طلب حذف المنشأة.")
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", "يرجى كتابة سبب طلب حذف المنشأة.")
		return
	}

	if _, err := h.orgSvc.RequestOrganizationDeletion(ctx, actor.OrganizationID, actor.UserID, reason); err != nil {
		h.log.ErrorContext(ctx, "request organization deletion", "org_id", actor.OrganizationID, "error", err)
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", h.errorMessage(r, err))
		return
	}

	h.notifyOrgDeletionRequested(ctx, actor.OrganizationID, reason)
	h.redirectWithNotice(w, r, base+"#danger-zone", "success", "تم تقديم طلب حذف المنشأة بنجاح، وسيتم مراجعته من قبل إدارة المنصة.")
}

// OrganizationDeletionCancelSubmit cancels an open pending deletion request.
func (h *UIHandler) OrganizationDeletionCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := organizationProfileBase(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(base), http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	requestID, _ := strconv.ParseInt(r.PostFormValue("request_id"), 10, 64)
	if requestID <= 0 {
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", "معرف الطلب غير صالح.")
		return
	}

	if err := h.orgSvc.CancelOrganizationDeletion(ctx, actor.OrganizationID, requestID); err != nil {
		h.log.ErrorContext(ctx, "cancel organization deletion", "org_id", actor.OrganizationID, "request_id", requestID, "error", err)
		h.redirectWithNotice(w, r, base+"#danger-zone", "error", h.errorMessage(r, err))
		return
	}

	h.redirectWithNotice(w, r, base+"#danger-zone", "success", "تم إلغاء طلب حذف المنشأة بنجاح.")
}

// readProfileSectionForm reads only the keys the section owns.
//
// A field the form did not send is not put in the map at all, so a section
// cannot blank a column it does not display.
func (h *UIHandler) readProfileSectionForm(r *http.Request, section org.ProfileSection) (org.ProfileFields, error) {
	if section == org.SectionMedia {
		if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
			return nil, err
		}
	} else if err := r.ParseForm(); err != nil {
		return nil, err
	}

	fields := org.ProfileFields{}
	for _, key := range profileSectionFormKeys(section) {
		if _, ok := r.Form[key]; ok {
			fields[key] = strings.TrimSpace(r.FormValue(key))
		}
	}

	if section == org.SectionMedia {
		fields["image"] = readProfileUpload(r, "logo_file")
		fields["coverage_image"] = readProfileUpload(r, "coverage_file")
	}
	return fields, nil
}

func profileSectionFormKeys(section org.ProfileSection) []string {
	switch section {
	case org.SectionIdentity:
		return []string{"legal_name", "trade_name_ar", "trade_name_en", "commercial_register", "tax_number"}
	case org.SectionLimits:
		return []string{"min_order_price", "max_order_price"}
	case org.SectionContact:
		return []string{"email", "phone", "address", "organization_number"}
	case org.SectionDescription:
		return []string{"description_ar", "description_en"}
	default:
		return nil
	}
}

// readProfileUpload returns "" when no file was chosen, which the repository
// reads as "keep the stored image".
func readProfileUpload(r *http.Request, field string) string {
	file, header, err := r.FormFile(field)
	if err != nil || file == nil {
		return ""
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		return ""
	}
	saved, err := saveUploadedBytes(data, header.Filename, "org")
	if err != nil {
		return ""
	}
	return saved
}
