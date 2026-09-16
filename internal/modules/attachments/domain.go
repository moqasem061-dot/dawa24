// Package attachments manages the lifecycle of all user and organization files,
// certificates, KYC documents, images, and attachments across the platform.
package attachments

import (
	"encoding/json"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// DocumentType represents the category of the uploaded document.
type DocumentType string

const (
	DocCommercialRegister  DocumentType = "commercial_register"
	DocTaxCard             DocumentType = "tax_card"
	DocPharmacistLicense   DocumentType = "pharmacist_license"
	DocPharmacyLicense     DocumentType = "pharmacy_license"
	DocNationalID          DocumentType = "national_id"
	DocPassport            DocumentType = "passport"
	DocBankCertificate     DocumentType = "bank_certificate"
	DocAuthorizationLetter DocumentType = "authorization_letter"
	DocSyndicateCard       DocumentType = "syndicate_card"
	DocAvatar              DocumentType = "avatar"
	DocOrgLogo             DocumentType = "organization_logo"
	DocProductImage        DocumentType = "product_image"
	DocReviewImage         DocumentType = "review_image"
	DocCV                  DocumentType = "cv"
	DocImportFile          DocumentType = "import_file"
	DocOther               DocumentType = "other"
)

// IsComplianceDocType returns true if the document type is a legal/compliance document for an organization.
func IsComplianceDocType(t DocumentType) bool {
	switch t {
	case DocCommercialRegister, DocTaxCard, DocPharmacistLicense,
		DocPharmacyLicense, DocNationalID, DocPassport,
		DocBankCertificate, DocAuthorizationLetter, DocSyndicateCard, DocOther:
		return true
	default:
		return false
	}
}

// DocumentStatus represents the administrative approval status.
type DocumentStatus string

const (
	StatusPending  DocumentStatus = "pending"
	StatusVerified DocumentStatus = "verified"
	StatusRejected DocumentStatus = "rejected"
)

// Document is the domain model matching platform_admin.documents.
type Document struct {
	ID             int64                  `json:"id"`
	PublicID       uuid.UUID              `json:"public_id"`
	OrganizationID *int64                 `json:"organization_id,omitempty"`
	UserID         *int64                 `json:"user_id,omitempty"`
	DocumentType   DocumentType           `json:"document_type"`
	FileURL        string                 `json:"file_url"`
	Title          string                 `json:"title"`
	StorageKey     string                 `json:"storage_key"`
	OriginalName   string                 `json:"original_name"`
	MimeType       string                 `json:"mime_type"`
	SizeBytes      int64                  `json:"size_bytes"`
	Status         DocumentStatus         `json:"status"`
	ReviewNotes    string                 `json:"review_notes"`
	ReviewedBy     *int64                 `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time             `json:"reviewed_at,omitempty"`
	Meta           map[string]interface{} `json:"meta"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	DeletedAt      *time.Time             `json:"deleted_at,omitempty"`
}

// DocumentRequestStatus represents the status of an administrative document request.
type DocumentRequestStatus string

const (
	DocReqPending   DocumentRequestStatus = "pending"
	DocReqSubmitted DocumentRequestStatus = "submitted"
	DocReqFulfilled DocumentRequestStatus = "fulfilled"
	DocReqCancelled DocumentRequestStatus = "cancelled"
)

// DocumentRequest is the domain model matching platform_admin.document_requests.
type DocumentRequest struct {
	ID             int64                 `json:"id"`
	OrganizationID int64                 `json:"organization_id"`
	OrgName        string                `json:"org_name,omitempty"`
	RequestedBy    *int64                `json:"requested_by,omitempty"`
	DocumentType   DocumentType          `json:"document_type"`
	Title          string                `json:"title"`
	Description    string                `json:"description"`
	DeadlineAt     time.Time             `json:"deadline_at"`
	Status         DocumentRequestStatus `json:"status"`
	SubmittedDocID *int64                `json:"submitted_doc_id,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// DaysRemaining returns the number of days until the deadline.
func (dr *DocumentRequest) DaysRemaining() int {
	diff := time.Until(dr.DeadlineAt)
	days := int(diff.Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// IsOverdue returns true if the deadline has passed without fulfillment.
func (dr *DocumentRequest) IsOverdue() bool {
	return dr.Status == DocReqPending && time.Now().After(dr.DeadlineAt)
}

// PresignRequest defines parameters sent by a client to request a direct upload URL.
type PresignRequest struct {
	DocumentType   DocumentType `json:"document_type"`
	OriginalName   string       `json:"original_name"`
	MimeType       string       `json:"mime_type"`
	SizeBytes      int64        `json:"size_bytes"`
	OrganizationID *int64       `json:"organization_id,omitempty"`
}

// PresignResult contains the pre-created document record and presigned upload URL.
type PresignResult struct {
	DocumentID int64     `json:"document_id"`
	PublicID   uuid.UUID `json:"public_id"`
	UploadURL  string    `json:"upload_url"`
	ExpiresAt  time.Time `json:"expires_at"`
	StorageKey string    `json:"storage_key"`
}

// Max file sizes per document category.
const (
	MaxDocumentSize = 10 * 1024 * 1024 // 10 MB
	MaxImageSize    = 5 * 1024 * 1024  // 5 MB
	MaxImportSize   = 50 * 1024 * 1024 // 50 MB
)

// Allowed MIME types by DocumentType.
var allowedMIMEs = map[DocumentType][]string{
	DocCommercialRegister:  {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocTaxCard:             {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocPharmacistLicense:   {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocPharmacyLicense:     {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocNationalID:          {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocPassport:            {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocBankCertificate:     {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocAuthorizationLetter: {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocSyndicateCard:       {"application/pdf", "image/jpeg", "image/png", "image/webp"},
	DocAvatar:              {"image/jpeg", "image/png", "image/webp", "image/gif"},
	DocOrgLogo:             {"image/jpeg", "image/png", "image/webp", "image/svg+xml"},
	DocProductImage:        {"image/jpeg", "image/png", "image/webp"},
	DocReviewImage:         {"image/jpeg", "image/png", "image/webp"},
	DocCV:                  {"application/pdf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	DocImportFile:          {"text/csv", "application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	DocOther:               {"application/pdf", "image/jpeg", "image/png", "image/webp"},
}

// ValidatePresignRequest enforces strict server-side MIME and size rules before generating a presigned URL.
func ValidatePresignRequest(req PresignRequest) error {
	if req.DocumentType == "" {
		return apperr.Validation("document.type_required", i18n.TDefault("w4s_mod.s_14_14"), map[string]string{"document_type": i18n.TDefault("w4s_mod.s_15_15")})
	}

	validMimes, ok := allowedMIMEs[req.DocumentType]
	if !ok {
		return apperr.Validation("document.type_invalid", i18n.TDefault("w4s_mod.s_16_16"), map[string]string{"document_type": i18n.TDefault("w4s_mod.s_17_17")})
	}

	mimeClean := strings.ToLower(strings.TrimSpace(req.MimeType))
	mimeMatched := false
	for _, m := range validMimes {
		if m == mimeClean {
			mimeMatched = true
			break
		}
	}
	if !mimeMatched {
		return apperr.Validation("document.mime_unsupported", fmt.Sprintf(i18n.TDefault("w4_mod.s_68"), req.MimeType), map[string]string{"mime_type": i18n.TDefault("w4s_mod.s_18_18")})
	}

	maxSize := int64(MaxDocumentSize)
	if req.DocumentType == DocAvatar || req.DocumentType == DocOrgLogo || req.DocumentType == DocProductImage || req.DocumentType == DocReviewImage {
		maxSize = MaxImageSize
	} else if req.DocumentType == DocImportFile {
		maxSize = MaxImportSize
	}

	if req.SizeBytes > maxSize {
		return apperr.Validation("document.size_exceeded", fmt.Sprintf(i18n.TDefault("w4_mod.d_69"), maxSize/(1024*1024)), map[string]string{"size_bytes": i18n.TDefault("w4s_mod.s_19_19")})
	}

	return nil
}

// GenerateStorageKey generates a secure, collision-free object key.
func GenerateStorageKey(docType DocumentType, orgID *int64, userID *int64, originalName string) string {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext == "" {
		ext = ".bin"
	}
	fileUUID := uuid.New().String()
	if orgID != nil && *orgID > 0 {
		return fmt.Sprintf("orgs/%d/%s/%s%s", *orgID, docType, fileUUID, ext)
	}
	if userID != nil && *userID > 0 {
		return fmt.Sprintf("users/%d/%s/%s%s", *userID, docType, fileUUID, ext)
	}
	return fmt.Sprintf("common/%s/%s%s", docType, fileUUID, ext)
}

// Requirement is one entry of the mandatory-document table (Rebuild V2 §4.2):
// a document type and whether the audience must hold it to trade.
type Requirement struct {
	DocType  DocumentType
	Required bool
}

// RequirementsFor returns the document requirements of an organization type.
// A vendor trades on the EDA license; every other type is treated as a
// customer (صيدلية) audience.
//
// The type is normalised first. Comparing the raw column against "vendor"
// meant a supplier row — the legacy spelling most supplier organizations
// actually carry — was asked for a pharmacist licence it has no way to hold.
func RequirementsFor(orgType string) []Requirement {
	if rbac.NormalizeOrgType(orgType) == rbac.OrgTypeVendor {
		return []Requirement{
			{DocCommercialRegister, false},
			{DocTaxCard, false},
			{DocPharmacyLicense, false},
			{DocAuthorizationLetter, false},
		}
	}
	return []Requirement{
		{DocCommercialRegister, false},
		{DocTaxCard, false},
		{DocPharmacyLicense, false},
		{DocPharmacistLicense, false},
		{DocAuthorizationLetter, false},
	}
}

// DocumentFilter for administrative cross-tenant search.
type DocumentFilter struct {
	OrganizationID *int64
	UserID         *int64
	DocumentType   *DocumentType
	Status         *DocumentStatus
	Search         string
	Limit          int
	Offset         int
}

// MetaJSON encodes meta into JSON bytes.
func (d *Document) MetaJSON() []byte {
	if d.Meta == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(d.Meta)
	if err != nil {
		return []byte("{}")
	}
	return b
}
