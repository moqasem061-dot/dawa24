package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/safe"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// documentsRedirect returns the actor to their documents screen after a form
// action, carrying the notice.
func (h *UIHandler) documentsRedirect(w http.ResponseWriter, r *http.Request, kind, message string) {
	target := "/customer/documents"
	if actor, ok := authctx.From(r.Context()); ok && actor.IsVendor() {
		target = "/vendor/documents"
	}
	h.redirectWithNotice(w, r, target, kind, message)
}

// OrganizationDocumentsPage renders the customer/vendor legal documents & licensing screen.
func (h *UIHandler) OrganizationDocumentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}

	data := &pages.OrganizationDocumentsData{}
	if h.attSvc != nil && orgID > 0 {
		sysCtx := database.AsSystem(ctx)
		docs, err := h.attSvc.ListByOrganization(sysCtx, orgID)
		reqs, _ := h.attSvc.ListDocumentRequests(sysCtx, actor, &orgID)
		if err != nil {
			h.log.ErrorContext(ctx, "load organization documents", "organization_id", orgID, "error", err)
			data.Error = i18n.T(lang, "docs.load_failed")
		} else {
			data = pages.BuildOrganizationDocumentsData(docs, reqs, actor.IsVendor())
		}
	}

	h.renderPage(ctx, w, "render organization documents", pages.OrganizationDocuments(lang, dir, data, actor.Permissions))
}

// OrganizationDocumentsUploadSubmit stores a freshly uploaded file and
// registers it as a pending document on the actor's organization.
func (h *UIHandler) OrganizationDocumentsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || (orgID <= 0 && !actor.IsPlatformAdmin()) {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.org_auth_required"))
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.service_unavailable"))
		return
	}

	docTypeStr := strings.TrimSpace(r.PostFormValue("document_type"))
	if docTypeStr == "" {
		docTypeStr = strings.TrimSpace(r.FormValue("document_type"))
	}
	docType := attachments.DocumentType(docTypeStr)
	if docType == "" {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.type_required"))
		return
	}

	// Try standard field "file", then fallback to "document_file" or "file_upload"
	formKeys := []string{"file", "document_file", "file_upload", "doc"}
	var fileURL, originalName string
	var uploadErr error

	for _, k := range formKeys {
		fileURL, originalName, uploadErr = saveUploadedFileMeta(r, k, "documents")
		if uploadErr == nil && fileURL != "" {
			break
		}
	}

	if fileURL == "" {
		errMsg := i18n.T(lang, "docs.file_required")
		if uploadErr != nil {
			errMsg = fmt.Sprintf(i18n.T(lang, "docs.upload_failed_prefix"), uploadErr.Error())
		}
		h.documentsRedirect(w, r, "error", errMsg)
		return
	}

	sysCtx := database.AsSystem(ctx)
	uploadedDoc, err := h.attSvc.RegisterUpload(sysCtx, actor, docType, fileURL, originalName)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to register document upload", "error", err)
		h.documentsRedirect(w, r, "error", h.safeMessage(err, lang))
		return
	}

	// If replacement reason is provided, attach it as a note for platform admins
	replacementReason := strings.TrimSpace(r.PostFormValue("replacement_reason"))
	if replacementReason != "" && uploadedDoc != nil {
		adminActor := authctx.Actor{IsStaff: true, Role: "admin"}
		_ = h.attSvc.VerifyDocument(sysCtx, adminActor, uploadedDoc.ID, attachments.StatusPending, fmt.Sprintf(i18n.T(lang, "docs.replacement_reason_note"), replacementReason))
	}

	// If linked to an administrative document request, fulfill or submit it
	reqIDStr := strings.TrimSpace(r.PostFormValue("request_id"))
	if reqIDStr != "" && uploadedDoc != nil {
		if reqID, err := strconv.ParseInt(reqIDStr, 10, 64); err == nil && reqID > 0 {
			_ = h.attSvc.SubmitDocumentForRequest(sysCtx, reqID, uploadedDoc.ID)
		}
	}

	// Notify platform admins about the newly uploaded document / license
	if uploadedDoc != nil {
		uploadOrgID := actor.OrganizationID
		if uploadOrgID <= 0 {
			uploadOrgID = actor.OrgID
		}
		safe.Go(h.log, "notify-admin-doc-uploaded", func() {
			h.notifyAdminDocumentUploaded(context.Background(), uploadedDoc, uploadOrgID, originalName, replacementReason)
		})

		// Trigger background integrity and malware scan
		if h.docScanEnqueue != nil {
			_ = h.docScanEnqueue(context.Background(), uploadedDoc.ID, uploadedDoc.OrganizationID, uploadedDoc.StorageKey, uploadedDoc.MimeType)
		}
	}

	if replacementReason != "" {
		h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.replaced_success"))
	} else {
		h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.uploaded_success"))
	}
}

// OrganizationDocumentDeleteSubmit removes a document, restricted strictly to platform administrators.
func (h *UIHandler) OrganizationDocumentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsPlatformAdmin() {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.delete_admin_only"))
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("id")), 10, 64)
	if err != nil || id <= 0 {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.invalid_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.attSvc.Delete(sysCtx, actor, id); err != nil {
		h.documentsRedirect(w, r, "error", h.safeMessage(err, lang))
		return
	}

	h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.deleted_success"))
}

// saveUploadedFileMeta saves a multipart file and returns its public URL and
// the client-side file name (for the documents registry).
func saveUploadedFileMeta(r *http.Request, formKey, category string) (url, name string, err error) {
	url, err = saveUploadedFile(r, formKey, category)
	if err != nil || url == "" {
		return url, "", err
	}
	if file, header, ferr := r.FormFile(formKey); ferr == nil {
		defer file.Close()
		name = header.Filename
	}
	return url, name, nil
}

func docTypeLabelAr(docType attachments.DocumentType) string {
	switch docType {
	case attachments.DocCommercialRegister:
		return "السجل التجاري"
	case attachments.DocTaxCard:
		return "البطاقة الضريبية"
	case attachments.DocPharmacistLicense:
		return "ترخيص مزاولة المهنة للصيدلي"
	case attachments.DocPharmacyLicense:
		return "ترخيص المنشأة / الصيدلية"
	case attachments.DocNationalID:
		return "بطاقة الرقم القومي"
	case attachments.DocPassport:
		return "جواز السفر"
	case attachments.DocBankCertificate:
		return "شهادة الحساب البنكي والآيبان"
	case attachments.DocAuthorizationLetter:
		return "خطاب التفويض المعتمد"
	case attachments.DocSyndicateCard:
		return "كارنيه نقابة الصيادلة"
	default:
		return "مستند ترخيص رسمي"
	}
}

func (h *UIHandler) notifyAdminDocumentUploaded(ctx context.Context, doc *attachments.Document, orgID int64, originalName, replacementReason string) {
	if h.notifSvc == nil || doc == nil {
		return
	}
	orgName := h.resolveOrgName(ctx, orgID)
	if orgName == "" {
		orgName = fmt.Sprintf("منشأة #%d", orgID)
	}

	docName := docTypeLabelAr(doc.DocumentType)
	if originalName != "" {
		docName = fmt.Sprintf("%s (%s)", docName, originalName)
	}

	var title, body string
	if replacementReason != "" {
		title = fmt.Sprintf("طلب استبدال مستند: %s", orgName)
		body = fmt.Sprintf("قامت المنشأة %s بطلب استبدال مستند ترخيص (%s). سبب الاستبدال: %s", orgName, docName, replacementReason)
	} else {
		title = fmt.Sprintf("مستند ترخيص جديد قيد المراجعة: %s", orgName)
		body = fmt.Sprintf("قامت المنشأة %s برفع مستند ترخيص جديد (%s) للمراجعة والاعتماد.", orgName, docName)
	}

	h.dispatchAdminNotification(ctx, "hr.document.view", title, body)
}

// DocScanEnqueueFunc hands an asynchronous file integrity and malware scan job to the background worker.
type DocScanEnqueueFunc func(ctx context.Context, docID int64, orgID *int64, storageKey, mimeType string) error

// SetDocScanEnqueue registers the queue dispatcher for document security scans.
func (h *UIHandler) SetDocScanEnqueue(fn DocScanEnqueueFunc) {
	h.docScanEnqueue = fn
}

// RegisterCustomerSharedRoutes mounts Tier C customer audience-specific shared paths.
func (h *UIHandler) RegisterCustomerSharedRoutes(r chi.Router) {
	r.Get("/customer/documents", h.OrganizationDocumentsPage)
	r.Get("/customer/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/customer/documents/{id}/download", h.DocumentDownloadHandler)
	r.Post("/customer/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/customer/documents/delete", h.OrganizationDocumentDeleteSubmit)
}

// RegisterVendorSharedRoutes mounts Tier C vendor audience-specific shared paths.
func (h *UIHandler) RegisterVendorSharedRoutes(r chi.Router) {
	r.Get("/vendor/documents", h.OrganizationDocumentsPage)
	r.Get("/vendor/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/vendor/documents/{id}/download", h.DocumentDownloadHandler)
	r.Post("/vendor/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/vendor/documents/delete", h.OrganizationDocumentDeleteSubmit)
}

