package pages

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type AdminImageImportPhase string

const (
	AdminImagePhaseUpload     AdminImageImportPhase = "upload"
	AdminImagePhaseMapping    AdminImageImportPhase = "mapping"
	AdminImagePhaseProcessing AdminImageImportPhase = "processing"
	AdminImagePhaseCompleted  AdminImageImportPhase = "completed"
	AdminImagePhaseFailed     AdminImageImportPhase = "failed"
)

type AdminImageImportRow struct {
	Index          int    `json:"index"`
	SKU            string `json:"sku"`
	ImageURL       string `json:"image_url"`
	Status         string `json:"status"` // "success", "not_found", "download_failed", "invalid_url", "pending"
	ProductID      *int64 `json:"product_id,omitempty"`
	ProductName    string `json:"product_name,omitempty"`
	SavedImagePath string `json:"saved_image_path,omitempty"`
	ErrorMsg       string `json:"error_msg,omitempty"`
}

type AdminImageImportSession struct {
	ID             string                 `json:"id"`
	OrgID          int64                  `json:"org_id"`
	UserID         int64                  `json:"user_id"`
	Filename       string                 `json:"filename"`
	Phase          AdminImageImportPhase  `json:"phase"`
	Progress       int                    `json:"progress"`
	ProgressNote   string                 `json:"progress_note"`
	TotalRows      int                    `json:"total_rows"`
	SuccessRows    int                    `json:"success_rows"`
	NotFoundRows   int                    `json:"not_found_rows"`
	ErrorRows      int                    `json:"error_rows"`
	Headers        []string               `json:"headers,omitempty"`
	IdentifierType string                 `json:"identifier_type"` // "sku" or "barcode"
	DetectedSKUCol int                    `json:"detected_sku_col"`
	DetectedURLCol int                    `json:"detected_url_col"`
	SampleRows     [][]string             `json:"sample_rows,omitempty"`
	RawDataRows    [][]string             `json:"-"`
	Rows           []*AdminImageImportRow `json:"rows,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	ExpiresAt      time.Time              `json:"expires_at"`
}

type AdminProductImagesImportView struct {
	Session    *AdminImageImportSession
	Sessions   []*AdminImageImportSession
	NoticeType string
	NoticeMsg  string
	Actor      authctx.Actor
}
