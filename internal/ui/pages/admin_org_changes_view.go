package pages

import (
	"net/url"
	"strconv"
)

// orgChangesHref builds one tab's link while preserving active filters.
func orgChangesHref(v AdminOrgChangesView, status string) string {
	vals := orgChangesQuery(v, status)
	encoded := vals.Encode()
	if encoded == "" {
		return "/admin/organizations/change-requests"
	}
	return "/admin/organizations/change-requests?" + encoded
}

// orgChangesQuery builds the query parameters for tabs and pagination,
// preserving active filters across status tab changes and pagination.
func orgChangesQuery(v AdminOrgChangesView, status string) url.Values {
	vals := url.Values{}
	if status != "" {
		vals.Set("status", status)
	}
	if v.Section != "" {
		vals.Set("section", v.Section)
	}
	if v.Search != "" {
		vals.Set("q", v.Search)
	}
	if v.DateFrom != "" {
		vals.Set("from", v.DateFrom)
	}
	if v.DateTo != "" {
		vals.Set("to", v.DateTo)
	}
	if v.SelectedOrgID > 0 {
		vals.Set("org_id", strconv.FormatInt(v.SelectedOrgID, 10))
	}
	if v.PerPage > 0 && v.PerPage != 25 {
		vals.Set("limit", strconv.Itoa(v.PerPage))
	}
	return vals
}

// orgChangesResetHref builds a reset link retaining only the active tab status.
func orgChangesResetHref(status string) string {
	if status == "" {
		return "/admin/organizations/change-requests"
	}
	return "/admin/organizations/change-requests?status=" + url.QueryEscape(status)
}
