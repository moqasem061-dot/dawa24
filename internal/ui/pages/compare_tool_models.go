package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// CompareToolView contains state and configuration for the 3-column compare tool workspace.
type CompareToolView struct {
	Lang            string
	Dir             string
	Files           []*compare.CompareFile
	MaxAllowedFiles int
	NoticeType      string
	NoticeMsg       string
	Audience            string // "admin" or ""
	TargetOrgID         int64
	TargetOrgName       string
	UploadURL           string
	SavingProductsCount int
}

// IsAdmin reports whether this compare session is operated by staff on behalf of an org.
func (v CompareToolView) IsAdmin() bool {
	return v.Audience == "admin"
}

// EffectiveUploadURL returns the target form action URL for spreadsheet uploads.
func (v CompareToolView) EffectiveUploadURL() string {
	if v.UploadURL != "" {
		return v.UploadURL
	}
	if v.IsAdmin() && v.TargetOrgID > 0 {
		return fmt.Sprintf("/admin/organizations/import/%d/temp-warehouse/upload", v.TargetOrgID)
	}
	return "/compare/upload"
}

// SubtabURL generates navigation links preserving target organization context in admin mode.
func (v CompareToolView) SubtabURL(base string) string {
	if v.IsAdmin() && v.TargetOrgID > 0 {
		return fmt.Sprintf("%s?org_id=%d", base, v.TargetOrgID)
	}
	return base
}
