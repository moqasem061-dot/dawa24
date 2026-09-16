package productmatch

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Pricing, stock and flags.
//
// A supplier price list almost never states all three of the public price, the
// discount and the net. It states two and expects the reader to know the third,
// and which two it states differs by supplier. So the three are reconciled
// explicitly here, and the reconciliation is written onto the row, because a
// price a vendor cannot account for is a price they will not trust.

// maxStorablePriceMinor is the largest value a NUMERIC(12,2) price column can
// hold, in piastres: 9,999,999,999.99. Caught here rather than at the write,
// where the constraint would abort a whole batch over one bad cell.
const maxStorablePriceMinor int64 = 999999999999

// maxDiscountBps is a hundred percent. Anything above it is a mistyped cell.
const maxDiscountBps int64 = 10000

// readPricing reconciles whatever price columns the file supplied.
func (r *Reader) readPricing(out *Row, cells []string) {
	public := r.amount(out, cells, FieldPublicPrice)
	sell := r.amount(out, cells, FieldPrice)
	net := r.amount(out, cells, FieldNetPrice)
	cost := r.amount(out, cells, FieldCostPrice)

	// A "selling price to the pharmacy" is the net by another name; a lone price
	// column with a discount beside it is the public price by another name. Both
	// substitutions are recorded so the vendor can see which reading was used.
	switch {
	case net.IsZero() && sell.IsPositive() && public.IsPositive():
		net = sell
	case public.IsZero() && sell.IsPositive():
		public = sell
		out.PricingNote = "لا يوجد عمود لسعر الجمهور؛ تم اعتبار عمود سعر البيع هو السعر الأساسي."
	case public.IsZero() && net.IsPositive():
		public = net
		out.PricingNote = "لا يوجد عمود لسعر الجمهور؛ تم اعتبار السعر الصافي هو السعر الأساسي."
	}

	out.CostPrice = cost
	out.PublicPrice = public
	out.DiscountBps = r.readDiscount(out, cells, public)
	out.CostDiscountBps = r.readCostDiscount(out, cells)
	if out.CostPrice.IsZero() && out.CostDiscountBps > 0 && out.PublicPrice.IsPositive() {
		out.CostPrice = applyDiscount(out.PublicPrice, out.CostDiscountBps)
	}

	switch {
	case net.IsPositive():
		out.NetPrice = net
		r.checkDerivedNet(out, public, net)
	case public.IsPositive():
		out.NetPrice = applyDiscount(public, out.DiscountBps)
	}

	if out.NetPrice.Minor() > out.PublicPrice.Minor() && out.PublicPrice.IsPositive() {
		out.add(Issue{
			Row: out.Number, Field: FieldNetPrice, Column: r.header(FieldNetPrice),
			Value:    out.NetPrice.String(),
			Severity: SeverityWarning,
			Message:  "السعر الصافي أعلى من سعر الجمهور في هذا الصف؛ تحقق من قيم الأسعار.",
		})
	}
	if cost.IsPositive() && public.IsPositive() && cost.Minor() > public.Minor() {
		out.add(Issue{
			Row: out.Number, Field: FieldCostPrice, Column: r.header(FieldCostPrice),
			Value:    cost.String(),
			Severity: SeverityWarning,
			Message:  "سعر التكلفة أعلى من سعر الجمهور في هذا الصف.",
		})
	}
}

// readDiscount resolves the discount to basis points from whichever of the
// three possible statements the file made.
func (r *Reader) readDiscount(out *Row, cells []string, base money.Amount) int64 {
	if raw := r.cell(cells, FieldDiscountPct); raw != "" {
		d, err := sheet.Coerce(raw)
		switch {
		case err == sheet.ErrNoValue:
		case err != nil:
			out.add(Issue{
				Row: out.Number, Field: FieldDiscountPct, Column: r.header(FieldDiscountPct),
				Value: raw, Severity: SeverityWarning,
				Message: "تعذر قراءة نسبة الخصم كرقم؛ تم تجاهلها لهذا الصف.",
			})
		default:
			// A cell written "0.32" in a column of 32s means thirty-two percent,
			// not a third of one percent. Only a value at or below one in a
			// column the analyser judged to be percentages is read that way, and
			// that judgement is not available per row — so the safer reading is
			// to take the number at face value and flag the impossible ones.
			bps := int64(d.Float*100 + 0.5)
			switch {
			case bps < 0:
				out.add(Issue{
					Row: out.Number, Field: FieldDiscountPct, Column: r.header(FieldDiscountPct),
					Value: raw, Severity: SeverityWarning,
					Message: "نسبة الخصم سالبة؛ تم تجاهلها لهذا الصف.",
				})
			case bps > maxDiscountBps:
				out.add(Issue{
					Row: out.Number, Field: FieldDiscountPct, Column: r.header(FieldDiscountPct),
					Value: raw, Severity: SeverityWarning,
					Message: fmt.Sprintf("نسبة الخصم «%s» أكبر من 100%%؛ تم تجاهلها لهذا الصف.", raw),
				})
			default:
				return bps
			}
		}
	}

	if amt := r.amount(out, cells, FieldDiscountAmt); amt.IsPositive() && base.IsPositive() {
		if amt.Minor() >= base.Minor() {
			out.add(Issue{
				Row: out.Number, Field: FieldDiscountAmt, Column: r.header(FieldDiscountAmt),
				Value: amt.String(), Severity: SeverityWarning,
				Message: "قيمة الخصم تساوي السعر أو تزيد عليه؛ تم تجاهلها لهذا الصف.",
			})
			return 0
		}
		return amt.Minor() * maxDiscountBps / base.Minor()
	}
	return 0
}

// readCostDiscount resolves the vendor's purchase discount to basis points.
func (r *Reader) readCostDiscount(out *Row, cells []string) int64 {
	if raw := r.cell(cells, FieldCostDiscountPct); raw != "" {
		d, err := sheet.Coerce(raw)
		switch {
		case err == sheet.ErrNoValue:
		case err != nil:
			out.add(Issue{
				Row: out.Number, Field: FieldCostDiscountPct, Column: r.header(FieldCostDiscountPct),
				Value: raw, Severity: SeverityWarning,
				Message: "تعذر قراءة نسبة خصم التكلفة كرقم؛ تم تجاهلها لهذا الصف.",
			})
		default:
			bps := int64(d.Float*100 + 0.5)
			switch {
			case bps < 0:
				out.add(Issue{
					Row: out.Number, Field: FieldCostDiscountPct, Column: r.header(FieldCostDiscountPct),
					Value: raw, Severity: SeverityWarning,
					Message: "نسبة خصم التكلفة سالبة؛ تم تجاهلها لهذا الصف.",
				})
			case bps > maxDiscountBps:
				out.add(Issue{
					Row: out.Number, Field: FieldCostDiscountPct, Column: r.header(FieldCostDiscountPct),
					Value: raw, Severity: SeverityWarning,
					Message: fmt.Sprintf("نسبة خصم التكلفة «%s» أكبر من 100%%؛ تم تجاهلها لهذا الصف.", raw),
				})
			default:
				return bps
			}
		}
	}
	return 0
}

// checkDerivedNet compares a stated net against the one the discount implies.
//
// Disagreement means one of the three columns was read as the wrong field, and
// it is far better caught here — per row, with both numbers quoted — than by a
// pharmacy noticing their invoice does not match the price list.
func (r *Reader) checkDerivedNet(out *Row, public, net money.Amount) {
	if out.DiscountBps == 0 || !public.IsPositive() || !net.IsPositive() {
		return
	}
	want := applyDiscount(public, out.DiscountBps)
	diff := want.Minor() - net.Minor()
	if diff < 0 {
		diff = -diff
	}
	// One percent of the net absorbs piastre rounding; more than that is a
	// different number, not a rounder one.
	if diff*100 <= net.Minor() {
		return
	}
	out.add(Issue{
		Row: out.Number, Field: FieldNetPrice, Column: r.header(FieldNetPrice),
		Value: net.String(), Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"السعر الصافي المذكور (%s) لا يطابق الناتج من سعر الجمهور بعد الخصم (%s).",
			net.String(), want.String()),
	})
}

// applyDiscount returns base less the given basis points.
func applyDiscount(base money.Amount, bps int64) money.Amount {
	if bps <= 0 {
		return base
	}
	out, err := base.Sub(base.ApplyPercent(bps))
	if err != nil {
		return base
	}
	return out
}

// amount reads one money column, refusing what a NUMERIC(12,2) cannot hold.
func (r *Reader) amount(out *Row, cells []string, f Field) money.Amount {
	raw := r.cell(cells, f)
	if raw == "" {
		return money.Zero
	}
	d, err := sheet.Coerce(raw)
	switch {
	case err == sheet.ErrNoValue:
		return money.Zero
	case err != nil:
		out.add(Issue{
			Row: out.Number, Field: f, Column: r.header(f), Value: raw,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("تعذر قراءة القيمة «%s» كرقم؛ تم تجاهلها.", raw),
		})
		return money.Zero
	}

	amt, err := money.Parse(d.Canonical)
	if err != nil {
		out.add(Issue{
			Row: out.Number, Field: f, Column: r.header(f), Value: raw,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("القيمة «%s» خارج النطاق المسموح به؛ تم تجاهلها.", raw),
		})
		return money.Zero
	}
	switch {
	case amt.IsNegative():
		out.add(Issue{
			Row: out.Number, Field: f, Column: r.header(f), Value: raw,
			Severity: SeverityWarning,
			Message:  "القيمة سالبة ولا تصلح كسعر؛ تم تجاهلها.",
		})
		return money.Zero
	case amt.Minor() > maxStorablePriceMinor:
		out.add(Issue{
			Row: out.Number, Field: f, Column: r.header(f), Value: raw,
			Severity: SeverityError,
			Message:  "تم رفض الصف: قيمة السعر تتجاوز الحد الأقصى المسموح (9,999,999,999.99).",
		})
		return money.Zero
	}
	if d.Rounded {
		out.add(Issue{
			Row: out.Number, Field: f, Column: r.header(f), Value: raw,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("تم تقريب القيمة إلى منزلتين عشريتين (%s).", amt.String()),
		})
	}
	return amt
}

// readStock fills the inventory fields.
func (r *Reader) readStock(out *Row, cells []string) {
	if raw := r.cell(cells, FieldQuantity); raw != "" {
		n, err := sheet.CoerceInt(raw)
		switch {
		case err == sheet.ErrNoValue:
		case err != nil:
			out.add(Issue{
				Row: out.Number, Field: FieldQuantity, Column: r.header(FieldQuantity),
				Value: raw, Severity: SeverityWarning,
				Message: fmt.Sprintf("تعذر قراءة الكمية «%s» كرقم؛ لن يتم تحديث رصيد هذا الصنف.", raw),
			})
		case n < 0:
			out.add(Issue{
				Row: out.Number, Field: FieldQuantity, Column: r.header(FieldQuantity),
				Value: raw, Severity: SeverityWarning,
				Message: "الرصيد سالب؛ تم اعتباره صفراً.",
			})
			out.Quantity, out.HasQuantity = 0, true
		default:
			out.Quantity, out.HasQuantity = int(n), true
		}
	} else if r.has(FieldQuantity) && r.opts.BlankQuantityIsZero {
		out.Quantity, out.HasQuantity = 0, true
	}

	if n, ok := r.count(out, cells, FieldMinOrderQty); ok && n > 0 {
		out.MinOrderQty = n
	}
	if n, ok := r.count(out, cells, FieldMinThreshold); ok && n >= 0 {
		out.MinThreshold = n
	}
	if out.MinOrderQty <= 0 {
		out.MinOrderQty = 1
	}

	out.BatchNumber = r.cell(cells, FieldBatchNumber)
	out.Warehouse = r.cell(cells, FieldWarehouse)
	out.Branch = r.cell(cells, FieldBranch)
	r.readExpiry(out, cells)
}

// count reads a small whole number column.
func (r *Reader) count(out *Row, cells []string, f Field) (int, bool) {
	raw := r.cell(cells, f)
	if raw == "" {
		return 0, false
	}
	n, err := sheet.CoerceInt(raw)
	if err != nil {
		if err != sheet.ErrNoValue {
			out.add(Issue{
				Row: out.Number, Field: f, Column: r.header(f), Value: raw,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("تعذر قراءة القيمة «%s» كعدد صحيح؛ تم تجاهلها.", raw),
			})
		}
		return 0, false
	}
	return int(n), true
}

// readExpiry reads and sanity-checks the expiry date.
func (r *Reader) readExpiry(out *Row, cells []string) {
	raw := r.cell(cells, FieldExpiryDate)
	if raw == "" {
		return
	}
	res, err := sheet.CoerceDate(raw)
	if err != nil {
		if err != sheet.ErrNoValue {
			out.add(Issue{
				Row: out.Number, Field: FieldExpiryDate, Column: r.header(FieldExpiryDate),
				Value: raw, Severity: SeverityWarning,
				Message: fmt.Sprintf("تعذر قراءة «%s» كتاريخ صلاحية؛ تم تجاهله.", raw),
			})
		}
		return
	}

	t := res.Time
	out.ExpiryDate = &t
	if res.DayMonthSwapped {
		out.add(Issue{
			Row: out.Number, Field: FieldExpiryDate, Column: r.header(FieldExpiryDate),
			Value: raw, Severity: SeverityInfo,
			Message: fmt.Sprintf("التاريخ «%s» يحتمل قراءتين؛ تمت قراءته يوم/شهر/سنة (%s).",
				raw, t.Format("2006-01-02")),
		})
	}
	if t.Before(r.opts.Now) {
		severity, message := SeverityWarning, "تاريخ الصلاحية منتهٍ بالفعل."
		if r.opts.RejectExpired {
			severity = SeverityError
			message = "تم رفض الصف: تاريخ الصلاحية منتهٍ بالفعل ولا يمكن عرض الصنف للبيع."
		}
		out.add(Issue{
			Row: out.Number, Field: FieldExpiryDate, Column: r.header(FieldExpiryDate),
			Value: t.Format("2006-01-02"), Severity: severity, Message: message,
		})
	}
}

// readFlags reads the boolean and status columns.
func (r *Reader) readFlags(out *Row, cells []string) {
	if raw := r.cell(cells, FieldStatus); raw != "" {
		switch {
		case inSet(yesWords, raw):
			out.Status = "active"
		case inSet(noWords, raw):
			out.Status = "inactive"
		case inSet(statusWords, raw):
			out.Status = "pending"
		default:
			out.add(Issue{
				Row: out.Number, Field: FieldStatus, Column: r.header(FieldStatus),
				Value: raw, Severity: SeverityWarning,
				Message: "حالة الصنف غير معروفة؛ تم الإبقاء على الحالة الافتراضية.",
			})
		}
	}
	if raw := r.cell(cells, FieldNegotiable); raw != "" {
		switch {
		case inSet(yesWords, raw):
			yes := true
			out.Negotiable = &yes
		case inSet(noWords, raw):
			no := false
			out.Negotiable = &no
		}
	}
}
