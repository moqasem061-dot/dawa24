package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Rows per printed A4 sheet (277mm of height inside the 10mm page margins),
// measured by printing to PDF with the print stylesheet: a row is 6.5mm, or
// 11.1mm when the item name wraps to a second line. The budgets hold when
// every name wraps, with a few millimetres to spare for print engines:
//
//	first sheet:  16 two-line rows measured 281mm, so 15 (≈270mm)
//	later sheets: 21 two-line rows measured 285mm, so 19 (≈262mm)
//	closing:      totals, signatures and disclaimer take ≈50mm
const (
	invoiceRowsFirstPage    = 15
	invoiceRowsNextPage     = 19
	invoiceRowsLastSingle   = 10 // one sheet: header + parties + totals
	invoiceRowsLastNextPage = 15 // final continuation sheet: totals
	invoiceRowsMinLastPage  = 3  // never a totals sheet with a lone row
)

// invoicePage is one printed sheet of the A4 invoice.
type invoicePage struct {
	Number, Count int
	First, Last   bool
	// Offset is the index of the sheet's first row within the invoice, for
	// the running row number.
	Offset int
	Lines  []*billing.PrintableInvoiceLine
	// CarriedIn is the sum of every line on earlier sheets; CarriedOut adds
	// this sheet's lines. Printed as "رصيد منقول" so each sheet reconciles.
	CarriedIn, CarriedOut money.Amount
}

// paginateInvoice splits the lines into sheets. An invoice with no lines is
// one sheet.
func paginateInvoice(lines []*billing.PrintableInvoiceLine) []invoicePage {
	var chunks [][]*billing.PrintableInvoiceLine
	for rest, capacity := lines, invoiceRowsFirstPage; len(rest) > 0; capacity = invoiceRowsNextPage {
		n := min(capacity, len(rest))
		chunks = append(chunks, rest[:n])
		rest = rest[n:]
	}
	if len(chunks) == 0 {
		chunks = [][]*billing.PrintableInvoiceLine{nil}
	}

	// The last sheet also carries the totals. When its rows leave no room,
	// its final rows move to one more sheet that ends the invoice.
	lastCap := invoiceRowsLastNextPage
	if len(chunks) == 1 {
		lastCap = invoiceRowsLastSingle
	}
	if last := chunks[len(chunks)-1]; len(last) > lastCap {
		move := min(invoiceRowsMinLastPage, len(last)-1)
		chunks[len(chunks)-1] = last[:len(last)-move]
		chunks = append(chunks, last[len(last)-move:])
	}

	pages := make([]invoicePage, len(chunks))
	offset, carried := 0, money.FromMinor(0)
	for i, c := range chunks {
		out := carried
		for _, l := range c {
			if l != nil {
				out, _ = out.Add(l.TotalPrice)
			}
		}
		pages[i] = invoicePage{
			Number: i + 1, Count: len(chunks),
			First: i == 0, Last: i == len(chunks)-1,
			Offset: offset, Lines: c,
			CarriedIn: carried, CarriedOut: out,
		}
		offset += len(c)
		carried = out
	}
	return pages
}
