package http

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type apiCheckoutLine struct {
	commerce.CheckoutLineItem
	Discount         *money.Amount `json:"discount_amount"`
	ListPrice        *money.Amount `json:"list_price"`
	OriginalPrice    *money.Amount `json:"original_price"`
	OriginalDiscount *money.Amount `json:"original_discount"`
}

type apiCheckoutRequest struct {
	commerce.CheckoutInput
	Items       []apiCheckoutLine `json:"items"`
	ShippingFee *money.Amount     `json:"shipping_fee"`
}

func (request apiCheckoutRequest) input() commerce.CheckoutInput {
	input := request.CheckoutInput
	if request.ShippingFee != nil {
		input.ShippingFee = *request.ShippingFee
		input.SubmittedShippingFee = true
	}
	input.Items = make([]commerce.CheckoutLineItem, len(request.Items))
	for i, line := range request.Items {
		input.Items[i] = line.CheckoutLineItem
		if line.Discount != nil {
			input.Items[i].DiscountAmount = *line.Discount
			input.Items[i].SubmittedDiscount = true
		}
		if line.ListPrice != nil {
			input.Items[i].ListPrice = *line.ListPrice
			input.Items[i].SubmittedListPrice = true
		}
		if line.OriginalPrice != nil {
			input.Items[i].OriginalPrice = *line.OriginalPrice
			input.Items[i].SubmittedOriginalPrice = true
		}
		if line.OriginalDiscount != nil {
			input.Items[i].OriginalDiscount = *line.OriginalDiscount
			input.Items[i].SubmittedOriginalDiscount = true
		}
	}
	return input
}
