package pages

import "fmt"

// IsAdmin reports whether this ingest session is being operated by platform staff on behalf of an organization.
func (v VendorImportView) IsAdmin() bool {
	return v.Audience == "admin"
}

// UploadPath returns the multipart upload endpoint corresponding to the operating audience.
func (v VendorImportView) UploadPath() string {
	if v.IsAdmin() && v.TargetOrgID > 0 {
		return fmt.Sprintf("/admin/organizations/import/%d/ingest/upload", v.TargetOrgID)
	}
	return "/vendor/ingest/upload"
}

// BaseIngestPath returns the base path of the ingest tool.
func (v VendorImportView) BaseIngestPath() string {
	if v.IsAdmin() && v.TargetOrgID > 0 {
		return fmt.Sprintf("/admin/organizations/import/%d/ingest", v.TargetOrgID)
	}
	return "/vendor/ingest"
}

// SessionPath generates the contextual action URL for an active session stage.
func (v VendorImportView) SessionPath(publicID, action string) string {
	base := v.BaseIngestPath()
	if publicID == "" {
		return base
	}
	if action == "" {
		return fmt.Sprintf("%s/%s", base, publicID)
	}
	return fmt.Sprintf("%s/%s/%s", base, publicID, action)
}

// RowActionPath generates the action URL for a specific row in the review stage.
func (v VendorImportView) RowActionPath(rowID int64, action string) string {
	sessID := ""
	if v.Session != nil {
		sessID = v.Session.PublicID
	}
	return fmt.Sprintf("%s/rows/%d/%s?match=%s&sort=%s&order=%s&page=%d&limit=%d&q=%s",
		v.SessionPath(sessID, ""),
		rowID, action,
		v.Filter.MatchLevel, v.Filter.SortBy, v.Filter.SortOrder,
		v.Page, v.PerPage, v.Filter.Search,
	)
}

// BulkActionPath generates the action URL for bulk operations in the review stage.
func (v VendorImportView) BulkActionPath() string {
	sessID := ""
	if v.Session != nil {
		sessID = v.Session.PublicID
	}
	return fmt.Sprintf("%s/rows/bulk?match=%s&sort=%s&order=%s&page=%d&limit=%d&q=%s",
		v.SessionPath(sessID, ""),
		v.Filter.MatchLevel, v.Filter.SortBy, v.Filter.SortOrder,
		v.Page, v.PerPage, v.Filter.Search,
	)
}

// BatchQuantityPath generates the action URL for applying a quantity batch.
func (v VendorImportView) BatchQuantityPath() string {
	sessID := ""
	if v.Session != nil {
		sessID = v.Session.PublicID
	}
	return fmt.Sprintf("%s/batch-quantity?match=%s&sort=%s&order=%s&page=%d&limit=%d&q=%s",
		v.SessionPath(sessID, ""),
		v.Filter.MatchLevel, v.Filter.SortBy, v.Filter.SortOrder,
		v.Page, v.PerPage, v.Filter.Search,
	)
}

// ReviewURL builds a link for tabs, sorting, and pagination in the review stage.
func (v VendorImportView) ReviewURL(match, sortBy, sortOrder string, page, limit int, search string) string {
	sessID := ""
	if v.Session != nil {
		sessID = v.Session.PublicID
	}
	u := fmt.Sprintf("%s?page=%d&limit=%d", v.SessionPath(sessID, ""), page, limit)
	if match != "" {
		u += "&match=" + match
	}
	if sortBy != "" {
		u += "&sort=" + sortBy
	}
	if sortOrder != "" {
		u += "&order=" + sortOrder
	}
	if search != "" {
		u += "&q=" + search
	}
	return u
}

