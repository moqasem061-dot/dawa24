package postgres

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestBuyerRequiresBranch(t *testing.T) {
	tests := []struct {
		name string
		q    catalog.BuyerOfferQuery
		want bool
	}{
		{name: "signed out visitor may browse", q: catalog.BuyerOfferQuery{}, want: false},
		{name: "buyer without receiving branch is blocked", q: catalog.BuyerOfferQuery{BuyerOrgID: 10}, want: true},
		{name: "buyer with receiving branch may query", q: catalog.BuyerOfferQuery{BuyerOrgID: 10, BuyerBranchID: 20}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buyerRequiresBranch(tt.q); got != tt.want {
				t.Fatalf("buyerRequiresBranch(%+v) = %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}
