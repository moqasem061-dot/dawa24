package compare

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The offers behind one count on the benchmark table.
//
// The table says "3 عروض أفضل بالسوق". The next question is always "which
// three, and at what price" — and answering it inside the table would mean
// carrying every competing offer for every one of a few thousand rows into the
// page. So the counts are links, and this recomputes one row's bucket on
// demand.

// BenchmarkBucket names one of the three columns.
type BenchmarkBucket string

const (
	// BucketBetter — offers cheaper than yours. This is the column that matters:
	// these are the suppliers a pharmacy would buy from instead of you.
	BucketBetter BenchmarkBucket = "better"
	// BucketEqual — offers within the equality tolerance of your net price.
	BucketEqual BenchmarkBucket = "equal"
	// BucketWorse — offers dearer than yours.
	BucketWorse BenchmarkBucket = "worse"
)

// ValidBenchmarkBucket reports whether a request names a real column, so a
// hand-edited query string cannot reach anything else.
func ValidBenchmarkBucket(s string) bool {
	switch BenchmarkBucket(s) {
	case BucketBetter, BucketEqual, BucketWorse:
		return true
	}
	return false
}

// BenchmarkOffer is one competing supplier's line, as the modal shows it.
type BenchmarkOffer struct {
	SupplierName string       `json:"supplier_name"`
	ProductName  string       `json:"product_name"`
	SKU          string       `json:"sku,omitempty"`
	Price        money.Amount `json:"price"`
	Discount     float64      `json:"discount"`
	NetPrice     money.Amount `json:"net_price"`
	// GapFromYours is this offer's net price minus yours: negative is cheaper.
	GapFromYours money.Amount `json:"gap_from_yours"`
	// Cheaper is true when this offer undercuts yours, so the view can say so
	// without re-deriving the sign.
	Cheaper bool `json:"cheaper"`
}

// BenchmarkOffers is the whole modal.
type BenchmarkOffers struct {
	Bucket       BenchmarkBucket  `json:"bucket"`
	ProductName  string           `json:"product_name"`
	SKU          string           `json:"sku,omitempty"`
	YourSupplier string           `json:"your_supplier"`
	YourPrice    money.Amount     `json:"your_price"`
	YourDiscount float64          `json:"your_discount"`
	YourNet      money.Amount     `json:"your_net"`
	Offers       []BenchmarkOffer `json:"offers"`
}

// BenchmarkRowOffers lists the market offers behind one count on one row.
//
// It repeats the classification the table did rather than trusting a number
// from the query string: the same tolerance, the same one-vote-per-supplier
// rule. If it did not, the modal could list four offers under a badge that
// said three.
func (s *Service) BenchmarkRowOffers(
	ctx context.Context, fileID, rowID int64, bucket BenchmarkBucket, orgID *int64,
) (*BenchmarkOffers, error) {
	if fileID <= 0 || rowID <= 0 || !ValidBenchmarkBucket(string(bucket)) {
		return nil, apperr.Validation("compare.invalid_offer_request",
			"طلب غير صالح لعرض تفاصيل المقارنة.", nil)
	}

	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("compare: load file: %w", err)
	}

	mine, err := s.repo.ListFileRows(ctx, fileID, 100000, 0)
	if err != nil {
		return nil, fmt.Errorf("compare: load file rows: %w", err)
	}
	var target *CompareFileRow
	for _, r := range mine {
		if r != nil && r.ID == rowID {
			target = r
			break
		}
	}
	if target == nil {
		return nil, apperr.NotFound("compare row")
	}

	net := target.PriceAfterDiscount
	if net.IsZero() && target.Price.IsPositive() {
		net = CalculatePriceAfterDiscount(target.Price, target.Discount)
	}

	out := &BenchmarkOffers{
		Bucket:       bucket,
		ProductName:  target.RawName,
		SKU:          target.SKU,
		YourSupplier: file.SupplierName,
		YourPrice:    target.Price,
		YourDiscount: round2(target.Discount),
		YourNet:      net,
		Offers:       []BenchmarkOffer{},
	}

	offers, err := s.repo.LoadMarketOffers(ctx, MarketScanOptions{
		OrganizationID:      orgID,
		ExcludeFileID:       fileID,
		ExcludeSupplierName: file.SupplierName,
	})
	if err != nil {
		return nil, fmt.Errorf("compare: load market offers: %w", err)
	}
	ds := BuildMarketDataset(offers)

	p, ok := ds.Lookup(target.MatchedProductID, target.RawName)
	if !ok || len(p.Offers) == 0 {
		return out, nil
	}

	tolerance := int64(float64(net.Minor()) * equalTolerancePercent / 100)
	seen := map[string]bool{}
	for _, o := range p.Offers {
		if file.SupplierName != "" && strings.EqualFold(strings.TrimSpace(o.SupplierName), strings.TrimSpace(file.SupplierName)) {
			continue
		}
		// One vote per supplier, exactly as the count was made.
		if name := strings.TrimSpace(o.SupplierName); name != "" {
			if seen[name] {
				continue
			}
			seen[name] = true
		}
		d := o.NetPrice.Minor() - net.Minor()
		var in bool
		switch {
		case d < -tolerance:
			in = bucket == BucketBetter
		case d > tolerance:
			in = bucket == BucketWorse
		default:
			in = bucket == BucketEqual
		}
		if !in {
			continue
		}
		out.Offers = append(out.Offers, BenchmarkOffer{
			SupplierName: o.SupplierName,
			ProductName:  o.ProductName,
			SKU:          o.SKU,
			Price:        o.Price,
			Discount:     round2(o.Discount),
			NetPrice:     o.NetPrice,
			GapFromYours: money.FromMinor(d),
			Cheaper:      d < 0,
		})
	}

	// Cheapest first: whatever bucket this is, the reader is comparing prices.
	sort.SliceStable(out.Offers, func(i, j int) bool {
		if out.Offers[i].NetPrice.Minor() != out.Offers[j].NetPrice.Minor() {
			return out.Offers[i].NetPrice.Minor() < out.Offers[j].NetPrice.Minor()
		}
		return out.Offers[i].Discount > out.Offers[j].Discount
	})
	return out, nil
}
