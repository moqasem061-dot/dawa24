package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Reading the special-offer form, once, for both create and edit.
//
// The two submit handlers had grown a copy each: the same fifteen fields parsed
// twice, and both parsed money with strconv.ParseFloat followed by
// int64(x*100). That is forbidden (AGENTS.md rule 1) and it is wrong —
// int64(29.99*100) is 2998 on a binary float. money.Parse reads the decimal
// string exactly.
//
// Neither copy validated anything, so a title-less offer with a 4000% discount
// and an end date before its start date was accepted and written.

// offerFormInput is one submitted special-offer form, already validated.
type offerFormInput struct {
	TitleAr        string
	TitleEn        string
	DescriptionAr  string
	Status         string
	BranchID       *int64
	DiscountPct    float64
	TotalPrice     money.Amount
	MinOrderAmount money.Amount
	StartDate      *time.Time
	EndDate        *time.Time
	Products       []*promo.SpecialOfferProduct
}

// offerFormError carries a translated message back to the submitting screen.
type offerFormError struct{ msg string }

func (e offerFormError) Error() string { return e.msg }

// readOfferForm parses and validates the special-offer form.
func readOfferForm(r *http.Request, lang string) (*offerFormInput, error) {
	in := &offerFormInput{}

	in.TitleAr = strings.TrimSpace(r.PostFormValue("title_ar"))
	if in.TitleAr == "" {
		return nil, offerFormError{i18n.T(lang, "vendor.offer.title_required")}
	}
	in.TitleEn = strings.TrimSpace(r.PostFormValue("title_en"))
	if in.TitleEn == "" {
		in.TitleEn = in.TitleAr
	}
	in.DescriptionAr = strings.TrimSpace(r.PostFormValue("description_ar"))

	in.Status = strings.TrimSpace(r.PostFormValue("status"))
	switch in.Status {
	case "active", "inactive", "draft":
	default:
		in.Status = "active"
	}

	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			in.BranchID = &bID
		}
	}

	if dStr := strings.TrimSpace(r.PostFormValue("discount_percentage")); dStr != "" {
		pct, err := strconv.ParseFloat(dStr, 64)
		if err != nil || pct < 0 || pct > 100 {
			return nil, offerFormError{i18n.T(lang, "vendor.offer.discount_range")}
		}
		in.DiscountPct = pct
	}

	var err error
	if in.TotalPrice, err = parseOfferAmount(r.PostFormValue("total_price"), lang); err != nil {
		return nil, err
	}
	if in.MinOrderAmount, err = parseOfferAmount(r.PostFormValue("min_order_amount"), lang); err != nil {
		return nil, err
	}

	if in.StartDate, err = parseOfferDate(r.PostFormValue("start_date"), lang); err != nil {
		return nil, err
	}
	if in.EndDate, err = parseOfferDate(r.PostFormValue("end_date"), lang); err != nil {
		return nil, err
	}
	if in.StartDate != nil && in.EndDate != nil && in.EndDate.Before(*in.StartDate) {
		return nil, offerFormError{i18n.T(lang, "vendor.offer.date_range")}
	}

	in.Products = readOfferProducts(r, lang)
	return in, nil
}

// parseOfferAmount reads a money field. Empty is zero, not an error: an offer
// with no bundle price is a plain percentage discount.
func parseOfferAmount(raw, lang string) (money.Amount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return money.Zero, nil
	}
	amount, err := money.Parse(raw)
	if err != nil || amount.IsNegative() {
		return money.Zero, offerFormError{i18n.T(lang, "vendor.offer.invalid_amount")}
	}
	return amount, nil
}

// parseOfferDate reads an optional yyyy-mm-dd field.
func parseOfferDate(raw, lang string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, offerFormError{i18n.T(lang, "vendor.offer.date_range")}
	}
	return &t, nil
}

// readOfferProducts collects the bundle lines. A variant the vendor does not
// own is rejected by the repository, which resolves product_id through a join
// scoped to the organization.
func readOfferProducts(r *http.Request, lang string) []*promo.SpecialOfferProduct {
	_ = lang
	var out []*promo.SpecialOfferProduct
	seen := make(map[int64]bool)
	for _, vIDStr := range r.Form["selected_variants"] {
		vID, err := strconv.ParseInt(strings.TrimSpace(vIDStr), 10, 64)
		if err != nil || vID <= 0 || seen[vID] {
			continue
		}
		seen[vID] = true

		qty, _ := strconv.Atoi(r.PostFormValue(fmt.Sprintf("qty_%d", vID)))
		if qty <= 0 {
			qty = 1
		}
		customPrice, _ := money.Parse(r.PostFormValue(fmt.Sprintf("custom_price_%d", vID)))
		pct, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("discount_pct_%d", vID)), 64)
		if pct < 0 || pct > 100 {
			pct = 0
		}

		out = append(out, &promo.SpecialOfferProduct{
			VariantID:          vID,
			Quantity:           qty,
			CustomPrice:        customPrice,
			DiscountPercentage: pct,
		})
	}
	return out
}

// applyTo folds the validated form onto an offer.
func (in *offerFormInput) applyTo(o *promo.SpecialOffer) {
	o.BranchID = in.BranchID
	o.Title = i18n.New(in.TitleAr, in.TitleEn)
	o.Description = i18n.New(in.DescriptionAr, in.DescriptionAr)
	o.DiscountPercentage = in.DiscountPct
	o.TotalPrice = in.TotalPrice
	o.MinOrderAmount = in.MinOrderAmount
	o.StartDate = in.StartDate
	o.EndDate = in.EndDate
	o.Status = in.Status
	o.AdminStatus = "pending"
	o.Products = in.Products
}
