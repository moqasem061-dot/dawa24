package pages

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func countPaymentsByStatus(items []*billing.AdminPaymentView, statuses ...string) int {
	cnt := 0
	for _, it := range items {
		for _, s := range statuses {
			if strings.EqualFold(it.Status, s) {
				cnt++
				break
			}
		}
	}
	return cnt
}

func calcPaymentsListTotal(items []*billing.AdminPaymentView) money.Amount {
	var totalMinor int64
	for _, it := range items {
		if strings.EqualFold(it.Status, "paid") || strings.EqualFold(it.Status, "completed") || strings.EqualFold(it.Status, "success") {
			totalMinor += it.Amount.Minor()
		}
	}
	return money.FromMinor(totalMinor)
}

func adminPaymentsPaginationQuery(q, status, method, dateFrom, dateTo string, orgID int64) url.Values {
	v := url.Values{}
	v.Set("tab", "payments")
	if q != "" {
		v.Set("q", q)
	}
	if status != "" {
		v.Set("status", status)
	}
	if method != "" {
		v.Set("method", method)
	}
	if dateFrom != "" {
		v.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		v.Set("date_to", dateTo)
	}
	if orgID > 0 {
		v.Set("org_id", strconv.FormatInt(orgID, 10))
	}
	return v
}
