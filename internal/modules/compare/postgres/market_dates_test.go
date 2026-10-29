package postgres

import (
	"reflect"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestMarketUploadDatesShareCountAndListingClauses(t *testing.T) {
	for _, filter := range []compare.MarketDiscountsFilter{
		{DateFrom: "2026-10-01"}, {DateTo: "2026-10-29"},
		{DateFrom: "2026-10-01", DateTo: "2026-10-29", Supplier: "supplier", MinPrice: priceBound("10")},
	} {
		countSQL, countArgs := buildMarketDiscountsCountQuery(filter)
		listSQL, listArgs, _, _ := buildMarketDiscountsQuery(filter)
		if !reflect.DeepEqual(countArgs, listArgs[:len(countArgs)]) {
			t.Fatal("date query arguments differ")
		}
		extra, _ := marketFilterClauses(filter)
		if !strings.Contains(countSQL, extra) || !strings.Contains(listSQL, extra) {
			t.Fatal("date clauses differ")
		}
		if !strings.Contains(extra, "AT TIME ZONE 'Africa/Cairo'") {
			t.Fatal("date must use Cairo time")
		}
		if filter.DateTo != "" && !strings.Contains(extra, "::date + 1") {
			t.Fatal("end date must include the entire day")
		}
	}
}
