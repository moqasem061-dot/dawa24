package ui

import (
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) renderMissingDocError(w http.ResponseWriter, r *http.Request, doc *attachments.Document, reason, lang, dir string) {
	actor, _ := authctx.From(r.Context())

	returnURL := "/customer/documents"
	if actor.IsPlatformAdmin() || actor.IsStaff {
		returnURL = "/admin/approvals?tab=documents"
	} else if actor.IsVendor() {
		returnURL = "/vendor/documents"
	}

	view := pages.DocumentUnavailableView{
		ReturnURL: returnURL,
		IsAdmin:   actor.IsPlatformAdmin() || actor.IsStaff,
	}

	if doc != nil {
		view.DocID = doc.ID
		view.DocTypeLabel = pages.FormatDocTypeLabel(doc.DocumentType)
		view.OriginalName = doc.OriginalName
		if view.OriginalName == "" {
			view.OriginalName = fmt.Sprintf("Document #%d", doc.ID)
		}
		view.UploadDate = doc.CreatedAt.Format("2006-01-02 03:04 PM")
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			view.OrgID = *doc.OrganizationID
			if h.orgSvc != nil {
				if orgObj, err := h.orgSvc.GetOrganization(r.Context(), *doc.OrganizationID); err == nil && orgObj != nil {
					view.OrgName = orgObj.LegalName
				}
			}
			if view.OrgName == "" {
				view.OrgName = fmt.Sprintf(i18n.T(lang, "docs.serve.org_fallback"), *doc.OrganizationID)
			}
		}
		switch doc.Status {
		case attachments.StatusVerified:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_verified")
		case attachments.StatusRejected:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_rejected")
		default:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_pending")
		}
	} else {
		view.DocTypeLabel = i18n.T(lang, "docs.serve.doc_unregistered")
		view.OriginalName = i18n.T(lang, "docs.serve.file_unavailable")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := pages.DocumentUnavailablePage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render document unavailable page", "error", err)
	}
}

// renderOfficialDocSVG dynamically generates an official SVG document badge/receipt.
func renderOfficialDocSVG(doc *attachments.Document) []byte {
	typeNameAr := i18n.T("ar", "doc.type.official_document")
	switch doc.DocumentType {
	case attachments.DocCommercialRegister:
		typeNameAr = i18n.T("ar", "doc.type.commercial_register")
	case attachments.DocTaxCard:
		typeNameAr = i18n.T("ar", "doc.type.tax_card")
	case attachments.DocPharmacistLicense:
		typeNameAr = i18n.T("ar", "doc.type.pharmacist_license")
	case attachments.DocPharmacyLicense:
		typeNameAr = i18n.TDefault("w4_ui.pharmacy_license_22")
	case attachments.DocNationalID:
		typeNameAr = i18n.TDefault("w4_ui.national_id_23")
	case attachments.DocPassport:
		typeNameAr = i18n.T("ar", "doc.type.passport")
	case attachments.DocBankCertificate:
		typeNameAr = i18n.TDefault("w4_ui.bank_certificate_24")
	case attachments.DocAuthorizationLetter:
		typeNameAr = i18n.T("ar", "doc.type.authorization_letter")
	case attachments.DocSyndicateCard:
		typeNameAr = i18n.T("ar", "doc.type.syndicate_card")
	default:
		typeNameAr = i18n.TDefault("w4_ui.s_73_73")
	}

	statusText := i18n.TDefault("w4_ui.s_74_74")
	statusColor := "#0284c7"
	statusBg := "#e0f2fe"
	if doc.Status == attachments.StatusVerified {
		statusText = i18n.TDefault("w4_ui.w4str_25_25")
		statusColor = "#16a34a"
		statusBg = "#dcfce7"
	} else if doc.Status == attachments.StatusRejected {
		statusText = i18n.TDefault("w4_ui.s_75_75")
		statusColor = "#dc2626"
		statusBg = "#fee2e2"
	}

	orgIDStr := i18n.TDefault("w4_ui.s_76_76")
	if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
		orgIDStr = fmt.Sprintf(i18n.TDefault("w4_ui.d_26"), *doc.OrganizationID)
	}

	dateStr := doc.CreatedAt.Format("2006-01-02 03:04 PM")
	filename := doc.OriginalName
	if filename == "" {
		filename = fmt.Sprintf("Document #%d", doc.ID)
	}

	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg width="800" height="520" viewBox="0 0 800 520" fill="none" xmlns="http://www.w3.org/2000/svg" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Noto Sans Arabic', sans-serif">
  <!-- Card Background -->
  <rect width="800" height="520" rx="16" fill="#FFFFFF"/>
  <rect x="1" y="1" width="798" height="518" rx="15" stroke="#E2E8F0" stroke-width="2"/>
  
  <!-- Top Header Bar -->
  <path d="M0 16C0 7.16344 7.16344 0 16 0H784C792.837 0 800 7.16344 800 16V80H0V16Z" fill="#0F172A"/>
  <text x="760" y="48" fill="#38BDF8" font-size="22" font-weight="800" text-anchor="end">DAWA24</text>
  <text x="40" y="48" fill="#94A3B8" font-size="14" font-weight="600">منصة دوا 24 لتداول وتوثيق الأدوية</text>
  
  <!-- Document Icon & Title -->
  <circle cx="720" cy="140" r="32" fill="#F1F5F9"/>
  <text x="720" y="148" font-size="24" text-anchor="middle">📑</text>
  
  <text x="670" y="132" fill="#0F172A" font-size="20" font-weight="800" text-anchor="end">%s</text>
  <text x="670" y="156" fill="#64748B" font-size="14" font-weight="600" text-anchor="end">الملف: %s</text>
  
  <!-- Status Badge -->
  <rect x="40" y="120" width="220" height="38" rx="19" fill="%s"/>
  <text x="150" y="144" fill="%s" font-size="13" font-weight="800" text-anchor="middle">%s</text>
  
  <!-- Details Container -->
  <rect x="40" y="195" width="720" height="200" rx="12" fill="#F8FAFC" stroke="#E2E8F0"/>
  
  <!-- Row 1 -->
  <text x="720" y="235" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">رقم المستند الرقمي:</text>
  <text x="450" y="235" fill="#0F172A" font-size="14" font-weight="700" text-anchor="end">#%d</text>
  
  <text x="320" y="235" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">المنشأة التابع لها:</text>
  <text x="80" y="235" fill="#0284C7" font-size="14" font-weight="800" text-anchor="start">%s</text>
  
  <!-- Divider -->
  <line x1="60" y1="260" x2="740" y2="260" stroke="#E2E8F0"/>
  
  <!-- Row 2 -->
  <text x="720" y="295" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">تاريخ الرفع والتسجيل:</text>
  <text x="450" y="295" fill="#0F172A" font-size="13" font-weight="700" text-anchor="end">%s</text>
  
  <text x="320" y="295" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">نوع التحقق القانوني:</text>
  <text x="80" y="295" fill="#0F172A" font-size="13" font-weight="700" text-anchor="start">مطابقة هيئة الدواء المصرية</text>
  
  <!-- Divider -->
  <line x1="60" y1="320" x2="740" y2="320" stroke="#E2E8F0"/>
  
  <!-- Row 3 -->
  <text x="720" y="355" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">ملاحظات التدقيق:</text>
  <text x="450" y="355" fill="#334155" font-size="13" font-weight="600" text-anchor="end">%s</text>

  <!-- Footer Seal -->
  <rect x="40" y="425" width="720" height="60" rx="8" fill="#F1F5F9"/>
  <text x="720" y="460" fill="#475569" font-size="12" font-weight="600" text-anchor="end">🔒 هذا المستند مسجل وموثق إلكترونياً بقاعدة بيانات منصة دوا 24 الرسمية.</text>
  <text x="60" y="460" fill="#10B981" font-size="13" font-weight="800" text-anchor="start">VERIFIED COMPLIANCE RECORD ✓</text>
</svg>`,
		typeNameAr,
		filename,
		statusBg,
		statusColor,
		statusText,
		doc.ID,
		orgIDStr,
		dateStr,
		doc.ReviewNotes,
	)

	return []byte(svg)
}
