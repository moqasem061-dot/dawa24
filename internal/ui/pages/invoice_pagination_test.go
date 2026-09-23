package pages

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func invoiceLines(n int) []*billing.PrintableInvoiceLine {
	out := make([]*billing.PrintableInvoiceLine, n)
	for i := range out {
		out[i] = &billing.PrintableInvoiceLine{TotalPrice: money.MustParse("10.50")}
	}
	return out
}

func TestPaginateInvoice(t *testing.T) {
	cases := []struct {
		lines int
		want  []int // rows per sheet
	}{
		{0, []int{0}},
		{5, []int{5}},
		{10, []int{10}},
		{11, []int{8, 3}},   // totals do not fit under 11 rows on sheet one
		{15, []int{12, 3}},  // a full first sheet
		{30, []int{15, 15}}, // the rest fits with the totals
		{34, []int{15, 16, 3}},
		{100, []int{15, 19, 19, 19, 19, 9}},
	}
	for _, c := range cases {
		pages := paginateInvoice(invoiceLines(c.lines))
		if len(pages) != len(c.want) {
			t.Fatalf("%d lines: %d sheets, want %d", c.lines, len(pages), len(c.want))
		}
		offset := 0
		for i, p := range pages {
			if len(p.Lines) != c.want[i] {
				t.Errorf("%d lines, sheet %d: %d rows, want %d", c.lines, i+1, len(p.Lines), c.want[i])
			}
			if p.Offset != offset || p.Number != i+1 || p.Count != len(pages) || p.First != (i == 0) || p.Last != (i == len(pages)-1) {
				t.Errorf("%d lines, sheet %d: bad metadata %+v", c.lines, i+1, p)
			}
			offset += len(p.Lines)
		}
		if offset != c.lines {
			t.Errorf("%d lines: sheets hold %d", c.lines, offset)
		}
	}
}

// The carried balance of the last sheet is the sum of every line, exactly.
func TestPaginateInvoiceCarriesTotals(t *testing.T) {
	pages := paginateInvoice(invoiceLines(42))
	if got := pages[0].CarriedOut.String(); got != "157.50" { // 15 × 10.50
		t.Errorf("sheet 1 carries %s, want 157.50", got)
	}
	if pages[1].CarriedIn != pages[0].CarriedOut {
		t.Error("sheet 2 must open with sheet 1's balance")
	}
	if got := pages[len(pages)-1].CarriedOut.String(); got != "441.00" { // 42 × 10.50
		t.Errorf("final balance %s, want 441.00", got)
	}
}
