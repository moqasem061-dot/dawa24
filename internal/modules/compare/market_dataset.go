package compare

import (
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The market, loaded whole, and grouped by product rather than by spelling.
//
// Both analytics screens used to build their own view of the market by calling
// ListMarketDiscounts with Limit: 50000. That method is the paginated feed
// behind /market-discounts and it clamps its limit to 100. So "the market" was
// the hundred most recently uploaded rows on the platform: تقرير ذكاء السوق
// reported an average discount over a hundred rows and called it the market,
// and مقارنة مع موردي السوق matched a supplier's fourteen hundred products
// against a hundred offers and reported the other thirteen hundred as حصري.
// That is the whole of why both screens looked broken, and no amount of UI
// work would have fixed either.
//
// The second half of the problem is what counts as "the same product". The old
// code grouped by three text keys derived from the raw name — a normalised
// form, a "core drug" key and a sorted-words key — and consulted them in turn.
// Text keys cannot see that فيتامين سي 1000 and Vitamin C 1000mg are one
// product, and they cannot see that two rows sharing a category word are not.
// The platform already has an engine that answers exactly this question, and
// its answer is stored: compare.file_rows.matched_product_id.
//
// So a group is a catalogue product where the row is matched, and the
// normalised name where it is not. Matched rows are the trustworthy half and
// they are kept separable, so every screen can say how much of its answer rests
// on the catalogue and how much on spelling.

// MarketOffer is one supplier's price for one product, as the market sees it.
type MarketOffer struct {
	RowID        int64
	FileID       int64
	SupplierName string
	ProductName  string
	SKU          string
	// ProductID is the catalogue product this row was matched to, or nil.
	ProductID *int64
	// Price is the list price before discount.
	Price money.Amount
	// Discount is the percentage off, 0–100.
	Discount float64
	// NetPrice is what a pharmacy actually pays.
	NetPrice money.Amount
}

// GroupKey identifies the product an offer is about.
//
// A catalogue id where one is known, and the shared product key otherwise. The
// prefixes keep the two spaces apart: without them a product whose name folds
// to "104" would collide with catalogue product 104.
//
// The fallback was the EXACT normalised name, which is the same thing as saying
// two suppliers only count as offering the same product when they spell it
// identically. They do not: one writes "زيرتك 10 مجم" and the next writes
// "Zyrtec 10mg", and the market benchmark counted those as two products with
// one offer each — so "how many suppliers carry this" and "what does the market
// charge" were both computed over a fragment of the market. productKeyOf folds
// the spelling, the alphabet and the way the dose is written, and still refuses
// to fold two different strengths together.
func (o MarketOffer) GroupKey() string {
	if o.ProductID != nil && *o.ProductID > 0 {
		return "p:" + itoa(*o.ProductID)
	}
	if k := productKeyOf(o.ProductName); k != "" {
		return k
	}
	return ""
}

// productKeyOf is the name-side group key, prefixed so it cannot collide with
// the catalogue-id space.
func productKeyOf(name string) string {
	if k := getCoreDrugMatchKey(name); k != "" {
		return "k:" + k
	}
	// Nothing identifying in the name. Fall back to the exact normalised form
	// rather than dropping the offer: a row this engine cannot read still has a
	// price somebody wants to see, and grouping it with its own exact duplicates
	// is better than grouping it with nothing.
	if n := normalizeProductText(name); n != "" {
		return "n:" + n
	}
	return ""
}

// Matched reports whether this offer is anchored to the shared catalogue.
func (o MarketOffer) Matched() bool { return o.ProductID != nil && *o.ProductID > 0 }

// MarketProduct is every offer the market carries for one product.
type MarketProduct struct {
	Key         string
	ProductID   *int64
	DisplayName string
	SKU         string
	Offers      []MarketOffer

	// Best is the offer a pharmacy should buy: lowest net price, ties broken by
	// the larger discount. Worst is the most expensive.
	//
	// Cheapest, not most-discounted. A 40% discount off an inflated list price
	// is not a saving, and ranking by discount is how a comparison tool
	// recommends the dearest supplier on the platform. The discount is reported
	// beside the price because buyers negotiate in discounts, but it never
	// decides.
	Best  MarketOffer
	Worst MarketOffer
}

// SupplierCount is how many distinct suppliers quote this product.
func (p *MarketProduct) SupplierCount() int {
	seen := make(map[string]bool, len(p.Offers))
	for _, o := range p.Offers {
		if s := strings.TrimSpace(o.SupplierName); s != "" {
			seen[s] = true
		}
	}
	return len(seen)
}

// Spread is what a pharmacy pays for buying from the wrong supplier, per unit.
func (p *MarketProduct) Spread() money.Amount {
	if p.Worst.NetPrice.Minor() <= p.Best.NetPrice.Minor() {
		return money.Zero
	}
	return money.FromMinor(p.Worst.NetPrice.Minor() - p.Best.NetPrice.Minor())
}

// SpreadPercent is the same figure as a share of the dearer price, which is how
// a buyer reads "how much am I overpaying".
func (p *MarketProduct) SpreadPercent() float64 {
	if p.Worst.NetPrice.Minor() <= 0 {
		return 0
	}
	return round2(float64(p.Spread().Minor()) / float64(p.Worst.NetPrice.Minor()) * 100)
}

// DiscountSpread is the difference between the best and worst discount terms.
func (p *MarketProduct) DiscountSpread() float64 {
	return round2(p.Best.Discount - p.Worst.Discount)
}

// MarketDataset is the whole comparable market, grouped.
type MarketDataset struct {
	Products map[string]*MarketProduct
	// Suppliers is every supplier with at least one usable offer.
	Suppliers []string
	// Offers, MatchedOffers and TotalRows describe the coverage of the answer,
	// so a screen can say "computed over 49,761 offers, 2,303 of them anchored
	// to the catalogue" instead of implying the whole market was understood.
	Offers        int
	MatchedOffers int
	// Rejected counts offers thrown out as unusable, and RejectedBySupplier
	// says whose. See MarketOffer.Usable: almost all of them are a discount
	// column holding something that is not a discount.
	Rejected           int
	RejectedBySupplier map[string]int
}

// WorstRejectedSuppliers names the files most in need of remapping, worst
// first, so the report can tell someone what to go and fix.
func (d *MarketDataset) WorstRejectedSuppliers(n int) []SupplierRejects {
	out := make([]SupplierRejects, 0, len(d.RejectedBySupplier))
	for name, count := range d.RejectedBySupplier {
		out = append(out, SupplierRejects{SupplierName: name, Rows: count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rows != out[j].Rows {
			return out[i].Rows > out[j].Rows
		}
		return out[i].SupplierName < out[j].SupplierName
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// SupplierRejects is one supplier's count of unusable rows.
type SupplierRejects struct {
	SupplierName string `json:"supplier_name"`
	Rows         int    `json:"rows"`
}

// Usable reports whether an offer can take part in a comparison.
//
// It rejects two things, and the second is the one that matters. A row with no
// list price is an obvious placeholder. A row whose NET price is zero — because
// its discount column says 100 or more — is a mapping error, and on the live
// data there are 6,182 of them: 12.5% of every priced row, including one
// supplier whose entire 1,365-row file reads as 100% off.
//
// Left in, those rows win every comparison they enter. The optimal basket cost
// collapses toward zero, the "best supplier" is whichever house has the most
// broken file, and the largest-saving table fills with rows reading "0.00 مقابل
// 1,382.00 — وفر 100%". Every figure on both screens was wrong in the same
// direction and for the same reason.
//
// They are not silently dropped either: BuildMarketDataset counts them and
// names the worst-affected suppliers, so the answer to "why is my file missing
// from the comparison?" is on the page rather than in a support ticket.
func (o MarketOffer) Usable() bool {
	if !o.Price.IsPositive() {
		return false
	}
	if o.Discount < 0 || o.Discount >= 100 {
		return false
	}
	return o.NetPrice.IsPositive()
}

// BuildMarketDataset groups a flat offer list by product.
func BuildMarketDataset(offers []MarketOffer) *MarketDataset {
	ds := &MarketDataset{
		Products:           make(map[string]*MarketProduct),
		RejectedBySupplier: map[string]int{},
	}
	suppliers := map[string]bool{}

	for _, o := range offers {
		if o.NetPrice.IsZero() && o.Price.IsPositive() {
			o.NetPrice = CalculatePriceAfterDiscount(o.Price, o.Discount)
		}
		if !o.Usable() {
			ds.Rejected++
			if s := strings.TrimSpace(o.SupplierName); s != "" {
				ds.RejectedBySupplier[s]++
			}
			continue
		}
		key := o.GroupKey()
		if key == "" {
			continue
		}
		ds.Offers++
		if o.Matched() {
			ds.MatchedOffers++
		}
		if s := strings.TrimSpace(o.SupplierName); s != "" {
			suppliers[s] = true
		}

		p, ok := ds.Products[key]
		if !ok {
			p = &MarketProduct{
				Key: key, ProductID: o.ProductID,
				DisplayName: o.ProductName, SKU: o.SKU,
				Best: o, Worst: o,
			}
			ds.Products[key] = p
		}
		p.Offers = append(p.Offers, o)
		if p.SKU == "" {
			p.SKU = o.SKU
		}
		// A matched row's name is the one to show: it is the catalogue's
		// product under a supplier's spelling, and any of them will do, but a
		// matched row at least proves the engine recognised it.
		if p.ProductID == nil && o.Matched() {
			p.ProductID = o.ProductID
			p.DisplayName = o.ProductName
		}
		if cheaper(o, p.Best) {
			p.Best = o
		}
		if cheaper(p.Worst, o) {
			p.Worst = o
		}
	}

	ds.Suppliers = make([]string, 0, len(suppliers))
	for s := range suppliers {
		ds.Suppliers = append(ds.Suppliers, s)
	}
	sort.Strings(ds.Suppliers)
	return ds
}

// Comparable lists the products more than one supplier quotes, which are the
// only ones a comparison has anything to say about.
func (d *MarketDataset) Comparable() []*MarketProduct {
	out := make([]*MarketProduct, 0, len(d.Products))
	for _, p := range d.Products {
		if p.SupplierCount() >= 2 {
			out = append(out, p)
		}
	}
	return out
}

// Exclusive lists the products only one supplier quotes.
func (d *MarketDataset) Exclusive() []*MarketProduct {
	out := make([]*MarketProduct, 0)
	for _, p := range d.Products {
		if p.SupplierCount() == 1 {
			out = append(out, p)
		}
	}
	return out
}

// Lookup finds the market's view of a product, by catalogue id first and by
// name second — the same two-step every screen needs and none should repeat.
func (d *MarketDataset) Lookup(productID *int64, name string) (*MarketProduct, bool) {
	if productID != nil && *productID > 0 {
		if p, ok := d.Products["p:"+itoa(*productID)]; ok {
			return p, true
		}
	}
	if k := productKeyOf(name); k != "" {
		if p, ok := d.Products[k]; ok {
			return p, true
		}
	}
	// The exact-name space is still consulted for offers whose names this
	// engine could not read, which productKeyOf files under "n:".
	if n := normalizeProductText(name); n != "" {
		if p, ok := d.Products["n:"+n]; ok {
			return p, true
		}
	}
	return nil, false
}

// cheaper reports whether a is the better buy than b: lower net price, and on a
// tie the larger discount, which is the better negotiating position for the
// same money.
func cheaper(a, b MarketOffer) bool {
	if a.NetPrice.Minor() != b.NetPrice.Minor() {
		return a.NetPrice.Minor() < b.NetPrice.Minor()
	}
	return a.Discount > b.Discount
}

func round2(v float64) float64 {
	return float64(int64(v*100+sign(v)*0.5)) / 100
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
