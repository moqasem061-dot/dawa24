package pages

import (
	"net/url"
	"strconv"
)

func vendorInvoicesSortURL(currentSort, currentOrder, col, q, status, dateFrom, dateTo string, branchID int64) string {
	nextOrder := "asc"
	if currentSort == col && currentOrder == "asc" {
		nextOrder = "desc"
	}
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if status != "" {
		v.Set("status", status)
	}
	if dateFrom != "" {
		v.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		v.Set("date_to", dateTo)
	}
	if branchID > 0 {
		v.Set("branch_id", strconv.FormatInt(branchID, 10))
	}
	v.Set("sort", col)
	v.Set("order", nextOrder)
	return "/invoices?" + v.Encode()
}

func vendorInvoicesPaginationQuery(q, status, dateFrom, dateTo, sortBy, sortOrder string, branchID int64) url.Values {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if status != "" {
		v.Set("status", status)
	}
	if dateFrom != "" {
		v.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		v.Set("date_to", dateTo)
	}
	if sortBy != "" {
		v.Set("sort", sortBy)
	}
	if sortOrder != "" {
		v.Set("order", sortOrder)
	}
	if branchID > 0 {
		v.Set("branch_id", strconv.FormatInt(branchID, 10))
	}
	return v
}
