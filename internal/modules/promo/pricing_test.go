package promo

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

var pct15 = 15.0

func TestEffectivePricePrecedence(t *testing.T) {
	list := money.MustParse("100.00")

	tests := []struct {
		name         string
		op           *OfferProduct
		o            *Offer
		wantFinal    string
		wantSource   DiscountSource
		wantDiscount string
		wantBPS      int64
	}{
		{
			name:         "no line and no offer falls back to list price",
			wantFinal:    "100.00",
			wantSource:   SourceNone,
			wantDiscount: "0.00",
		},
		{
			name: "custom price overrides everything",
			op: &OfferProduct{
				CustomPrice:          moneyPtr("85.00"),
				CustomDiscountAmount: moneyPtr("10.00"),
			},
			o:            &Offer{DiscountType: DiscountPercentage, DiscountValue: money.MustParse("50.00")},
			wantFinal:    "85.00",
			wantSource:   SourceCustomPrice,
			wantDiscount: "0.00",
		},
		{
			name: "custom fixed amount beats percent and offer",
			op: &OfferProduct{
				CustomDiscountAmount:  moneyPtr("10.00"),
				CustomDiscountPercent: &pct15,
			},
			o:            &Offer{DiscountType: DiscountFixed, DiscountValue: money.MustParse("5.00")},
			wantFinal:    "90.00",
			wantSource:   SourceCustomAmount,
			wantDiscount: "10.00",
		},
		{
			name:         "custom percentage beats offer discount",
			op:           &OfferProduct{CustomDiscountPercent: &pct15},
			o:            &Offer{DiscountType: DiscountFixed, DiscountValue: money.MustParse("25.00")},
			wantFinal:    "85.00",
			wantSource:   SourceCustomPercent,
			wantDiscount: "15.00",
			wantBPS:      1500,
		},
		{
			name:         "offer-level fixed discount",
			o:            &Offer{DiscountType: DiscountFixed, DiscountValue: money.MustParse("7.50")},
			wantFinal:    "92.50",
			wantSource:   SourceOffer,
			wantDiscount: "7.50",
		},
		{
			name:         "offer-level percentage discount uses value as basis points",
			o:            &Offer{DiscountType: DiscountPercentage, DiscountValue: money.MustParse("15.00")},
			wantFinal:    "85.00",
			wantSource:   SourceOffer,
			wantDiscount: "15.00",
			wantBPS:      1500,
		},

		{
			name:         "zombie offer with zero value yields list price",
			o:            &Offer{DiscountType: DiscountPercentage},
			wantFinal:    "100.00",
			wantSource:   SourceNone,
			wantDiscount: "0.00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bd := EffectivePrice(list, tt.op, tt.o)
			if s := got.String(); s != tt.wantFinal {
				t.Errorf("final = %s, want %s", s, tt.wantFinal)
			}
			if bd.Source != tt.wantSource {
				t.Errorf("source = %s, want %s", bd.Source, tt.wantSource)
			}
			if s := bd.DiscountAmount.String(); s != tt.wantDiscount {
				t.Errorf("discount amount = %s, want %s", s, tt.wantDiscount)
			}
			if bd.DiscountBPS != tt.wantBPS {
				t.Errorf("discount bps = %d, want %d", bd.DiscountBPS, tt.wantBPS)
			}
		})
	}
}

func TestEffectivePriceFloorsAtZero(t *testing.T) {
	list := money.MustParse("25.00")

	got, bd := EffectivePrice(list, &OfferProduct{CustomDiscountAmount: moneyPtr("500.00")}, nil)
	if s := got.String(); s != "0.00" {
		t.Errorf("fixed over-discount final = %s, want 0.00", s)
	}
	if bd.DiscountAmount.String() != "25.00" {
		t.Errorf("clamped discount = %s, want 25.00", bd.DiscountAmount.String())
	}

	got, _ = EffectivePrice(list, nil, &Offer{DiscountType: DiscountPercentage, DiscountValue: money.MustParse("9000.00")})
	if s := got.String(); s != "0.00" {
		t.Errorf("percent over-discount final = %s, want 0.00", s)
	}
}

func TestEffectivePriceRounding(t *testing.T) {
	list := money.MustParse("100.00")
	pct := 12.55
	got, bd := EffectivePrice(list, &OfferProduct{CustomDiscountPercent: &pct}, nil)
	if want := "87.45"; got.String() != want {
		t.Errorf("12.55%% of 100.00 = %s, want %s", got.String(), want)
	}
	if bd.DiscountBPS != 1255 {
		t.Errorf("bps = %d, want 1255", bd.DiscountBPS)
	}
}

func TestPercentToBPS(t *testing.T) {
	tests := []struct {
		pct  float64
		want int64
	}{
		{15.00, 1500},
		{12.55, 1255},
		{0.50, 50},
		{0, 0},
		{100.00, 10000},
	}
	for _, tt := range tests {
		if got := percentToBPS(tt.pct); got != tt.want {
			t.Errorf("percentToBPS(%v) = %d, want %d", tt.pct, got, tt.want)
		}
	}
}

func moneyPtr(s string) *money.Amount {
	a := money.MustParse(s)
	return &a
}
