package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

func (r *Reader) readSaleDiscount(out *Row, cells []string, public money.Amount) {
	raw := r.cell(cells, FieldNetPrice)
	decimal, parseErr := sheet.Coerce(raw)
	sale, amountErr := money.Parse(decimal.Canonical)
	public = r.salePublicPrice(cells, public)
	discount, discountErr := money.DiscountFromSalePrice(public, sale)
	if parseErr != nil || amountErr != nil || discountErr != nil || decimal.Percent || (!r.has(FieldPublicPrice) && !r.has(FieldPrice)) {
		out.add(Issue{Row: out.Number, Field: FieldNetPrice, Column: r.header(FieldNetPrice),
			Value: raw, Severity: SeverityError,
			Message: "vendor.pricing.invalid"})
		return
	}
	out.DiscountBps = discount.Minor()
	r.checkDerivedNet(out, public, sale)
	out.NetPrice = public.ApplyPercent(10000 - out.DiscountBps)
}

func (r *Reader) salePublicPrice(cells []string, fallback money.Amount) money.Amount {
	if !r.has(FieldPublicPrice) {
		return fallback
	}
	decimal, err := sheet.Coerce(r.cell(cells, FieldPublicPrice))
	if err != nil || decimal.Percent {
		return money.Zero
	}
	public, err := money.Parse(decimal.Canonical)
	if err != nil {
		return money.Zero
	}
	return public
}
