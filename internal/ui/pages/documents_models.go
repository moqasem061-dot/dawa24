package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// docTitlesAr is the UI-facing Arabic title per document type.
var docTitlesAr = map[attachments.DocumentType]string{
	attachments.DocCommercialRegister:  i18n.T("ar", "doc.type.commercial_register"),
	attachments.DocTaxCard:             i18n.T("ar", "doc.type.tax_card"),
	attachments.DocPharmacyLicense:     i18n.T("ar", "doc.type.pharmacy_license"),
	attachments.DocPharmacistLicense:   i18n.T("ar", "doc.type.pharmacist_license"),
	attachments.DocNationalID:          i18n.T("ar", "doc.type.national_id"),
	attachments.DocPassport:            i18n.T("ar", "doc.type.passport"),
	attachments.DocSyndicateCard:       i18n.T("ar", "doc.type.syndicate_card"),
	attachments.DocAuthorizationLetter: i18n.T("ar", "doc.type.authorization_letter"),
	attachments.DocBankCertificate:     i18n.T("ar", "doc.type.bank_certificate"),
	attachments.DocOther:               i18n.T("ar", "doc.type.other"),
}

// DocRequirement is one entry of the document requirement table.
type DocRequirement struct {
	DocType     attachments.DocumentType
	TitleAr     string
	Description string
	Required    bool
}

// docRequirements returns the audience requirement set with UI titles.
func docRequirements(vendor bool) []DocRequirement {
	if vendor {
		return []DocRequirement{
			{
				DocType:     attachments.DocCommercialRegister,
				TitleAr:     i18n.T("ar", "doc.req.vendor_cr_title"),
				Description: i18n.T("ar", "doc.req.vendor_cr_desc"),
				Required:    true,
			},
			{
				DocType:     attachments.DocTaxCard,
				TitleAr:     i18n.T("ar", "doc.req.vendor_tax_title"),
				Description: i18n.T("ar", "doc.req.vendor_tax_desc"),
				Required:    true,
			},
			{
				DocType:     attachments.DocPharmacyLicense,
				TitleAr:     i18n.T("ar", "doc.req.vendor_license_title"),
				Description: i18n.T("ar", "doc.req.vendor_license_desc"),
				Required:    true,
			},
			{
				DocType:     attachments.DocAuthorizationLetter,
				TitleAr:     i18n.T("ar", "doc.req.vendor_auth_title"),
				Description: i18n.T("ar", "doc.req.vendor_auth_desc"),
				Required:    false,
			},
		}
	}

	return []DocRequirement{
		{
			DocType:     attachments.DocPharmacyLicense,
			TitleAr:     i18n.T("ar", "doc.req.pharm_license_title"),
			Description: i18n.T("ar", "doc.req.pharm_license_desc"),
			Required:    true,
		},
		{
			DocType:     attachments.DocPharmacistLicense,
			TitleAr:     i18n.T("ar", "doc.req.pharmacist_title"),
			Description: i18n.T("ar", "doc.req.pharmacist_desc"),
			Required:    true,
		},
		{
			DocType:     attachments.DocCommercialRegister,
			TitleAr:     i18n.T("ar", "doc.req.pharm_cr_title"),
			Description: i18n.T("ar", "doc.req.pharm_cr_desc"),
			Required:    false,
		},
		{
			DocType:     attachments.DocTaxCard,
			TitleAr:     i18n.T("ar", "doc.req.vendor_tax_title"),
			Description: i18n.T("ar", "doc.req.pharm_tax_desc"),
			Required:    false,
		},
		{
			DocType:     attachments.DocAuthorizationLetter,
			TitleAr:     i18n.T("ar", "doc.req.pharm_auth_title"),
			Description: i18n.T("ar", "doc.req.pharm_auth_desc"),
			Required:    false,
		},
	}
}

// OrganizationDocumentsData backs the shared customer/vendor documents screen.
type OrganizationDocumentsData struct {
	IsVendor     bool
	Requirements []DocRequirement
	Docs         []*attachments.Document
	Requests     []*attachments.DocumentRequest
	Missing      []DocRequirement
	Error        string
}

// BuildOrganizationDocumentsData groups the org's documents by requirement
// and computes which mandatory ones are missing.
func BuildOrganizationDocumentsData(docs []*attachments.Document, requests []*attachments.DocumentRequest, vendor bool) *OrganizationDocumentsData {
	data := &OrganizationDocumentsData{
		IsVendor: vendor,
		Docs:     docs,
		Requests: requests,
	}
	data.Requirements = docRequirements(vendor)

	hasVerified := false
	for _, d := range docs {
		if d != nil && d.Status == attachments.StatusVerified && d.DeletedAt == nil {
			hasVerified = true
			break
		}
	}

	if !hasVerified {
		data.Missing = []DocRequirement{
			{TitleAr: i18n.TDefault("w4_ui.w4str_44_44")},
		}
	}
	return data
}

// LatestFor returns the newest document of the given type, or nil.
func (d *OrganizationDocumentsData) LatestFor(t attachments.DocumentType) *attachments.Document {
	var latest *attachments.Document
	for _, doc := range d.Docs {
		if doc == nil || doc.DocumentType != t || doc.DeletedAt != nil {
			continue
		}
		if latest == nil || doc.CreatedAt.After(latest.CreatedAt) {
			latest = doc
		}
	}
	return latest
}

// TotalRequirementsCount returns total requirements.
func (d *OrganizationDocumentsData) TotalRequirementsCount() int {
	return len(d.Requirements)
}

// VerifiedCount counts requirements that have an active verified document.
func (d *OrganizationDocumentsData) VerifiedCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusVerified {
			count++
		}
	}
	return count
}

// PendingCount counts requirements currently under administrative review.
func (d *OrganizationDocumentsData) PendingCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusPending {
			count++
		}
	}
	return count
}

// RejectedCount counts requirements that were rejected.
func (d *OrganizationDocumentsData) RejectedCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusRejected {
			count++
		}
	}
	return count
}

// CompletionPercentage returns integer percentage of verified documents against total requirements.
func (d *OrganizationDocumentsData) CompletionPercentage() int {
	if len(d.Requirements) == 0 {
		return 100
	}
	v := d.VerifiedCount()
	return (v * 100) / len(d.Requirements)
}

// ActiveDocRequests returns active (pending or submitted) requests from platform administration.
func (d *OrganizationDocumentsData) ActiveDocRequests() []*attachments.DocumentRequest {
	var active []*attachments.DocumentRequest
	for _, r := range d.Requests {
		if r != nil && (r.Status == attachments.DocReqPending || r.Status == attachments.DocReqSubmitted) {
			active = append(active, r)
		}
	}
	return active
}

// MissingTitles joins the missing requirements for the banner message.
func (d *OrganizationDocumentsData) MissingTitles() string {
	if len(d.Missing) == 0 {
		return ""
	}
	joined := ""
	for i, req := range d.Missing {
		if i > 0 {
			joined += i18n.TDefault("w4_ui.w4str_35_35")
		}
		joined += req.TitleAr
	}
	return joined
}
