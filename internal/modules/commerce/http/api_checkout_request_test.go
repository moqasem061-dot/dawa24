package http

import (
	"encoding/json"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestCheckoutRequestPreservesExplicitZeroDiscount(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		present    bool
	}{
		{"omitted", `{"items":[{"unit_price":"12.00","quantity":2}]}`, false},
		{"explicit zero", `{"items":[{"unit_price":"12.00","quantity":2,"discount_amount":"0.00"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request apiCheckoutRequest
			if err := json.Unmarshal([]byte(tc.body), &request); err != nil {
				t.Fatal(err)
			}
			input := request.input()
			if len(input.Items) != 1 || input.Items[0].SubmittedDiscount != tc.present || input.Items[0].UnitPrice != money.MustParse("12.00") {
				t.Fatalf("input: %+v", input)
			}
		})
	}
}

func TestCheckoutRequestPreservesOptionalPriceFields(t *testing.T) {
	var request apiCheckoutRequest
	if err := json.Unmarshal([]byte(`{"items":[{"unit_price":"12.00","quantity":2,"list_price":"0.00","original_price":"0.00","original_discount":"0.00"}]}`), &request); err != nil {
		t.Fatal(err)
	}
	line := request.input().Items[0]
	if !line.SubmittedListPrice || !line.SubmittedOriginalPrice || !line.SubmittedOriginalDiscount {
		t.Fatalf("missing supplied amount flags: %+v", line)
	}
}

func TestCheckoutRequestPreservesExplicitZeroShippingFee(t *testing.T) {
	var request apiCheckoutRequest
	if err := json.Unmarshal([]byte(`{"shipping_fee":"0.00","items":[]}`), &request); err != nil {
		t.Fatal(err)
	}
	if input := request.input(); !input.SubmittedShippingFee || !input.ShippingFee.IsZero() {
		t.Fatalf("shipping fee was not preserved: %+v", input)
	}
}
