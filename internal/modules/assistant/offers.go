package assistant

// OfferQuery asks what a buyer can order for one of its branches.
type OfferQuery struct {
	Search string
	// BranchID is a verified branch id, or zero for the caller's default
	// buying branch.
	BranchID       int64
	OnlyDiscounted bool
	Limit          int
}

// BuyableOffer is one supplier listing as the buying branch sees it right now.
// Orderable and MaxQuantity come from the same availability rule the cart and
// checkout apply, so the assistant cannot promise what checkout would refuse.
type BuyableOffer struct {
	VariantID   int64  `json:"-"`
	ProductID   int64  `json:"-"`
	Ref         string `json:"ref"`
	Product     string `json:"product"`
	Scientific  string `json:"scientific_name,omitempty"`
	Supplier    string `json:"supplier"`
	Price       string `json:"price"`
	PublicPrice string `json:"public_price,omitempty"`
	Discount    int    `json:"discount_percent,omitempty"`
	MinQuantity int    `json:"min_quantity,omitempty"`
	Orderable   bool   `json:"orderable"`
	MaxQuantity int    `json:"max_quantity,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Expiry      string `json:"expiry,omitempty"`
}

// OfferResult is a page of buyable offers for one branch.
type OfferResult struct {
	Branch string         `json:"branch"`
	Total  int            `json:"total"`
	Offers []BuyableOffer `json:"offers"`
}

// PromotionQuery asks which promotional offers (العروض والخصومات) a buyer can
// see for one of its branches.
type PromotionQuery struct {
	Search         string
	BranchID       int64 // verified branch id, or zero for the buying branch
	OnlyDiscounted bool
	Limit          int
}

// Promotion is one offer card as the buying branch sees it. Every promotion
// listed passes the platform's offer rule for that branch today; the ones it
// does not pass are not listed.
type Promotion struct {
	OfferID     int64  `json:"-"`
	Ref         string `json:"ref"`
	Title       string `json:"title"`
	Supplier    string `json:"supplier"`
	Description string `json:"description,omitempty"`
	Discount    string `json:"discount,omitempty"`
	BundlePrice string `json:"bundle_price,omitempty"`
	MinOrder    string `json:"min_order,omitempty"`
	Products    int    `json:"products"`
	Expires     string `json:"expires,omitempty"`
	Sponsored   bool   `json:"sponsored,omitempty"`
}

// PromotionResult is a page of promotions for one branch.
type PromotionResult struct {
	Branch     string      `json:"branch,omitempty"`
	Total      int         `json:"total"`
	Notice     string      `json:"notice,omitempty"`
	Promotions []Promotion `json:"promotions"`
}

// PromotionDetail is one promotion with its bundle and the verdict for the
// buying branch.
type PromotionDetail struct {
	Promotion
	Description string             `json:"description,omitempty"`
	Items       []PromotionProduct `json:"items"`
	Purchasable bool               `json:"purchasable"`
	Reason      string             `json:"reason,omitempty"`
}

// PromotionProduct is one line of a bundle.
type PromotionProduct struct {
	Product  string `json:"product"`
	Quantity int    `json:"quantity"`
	Price    string `json:"price,omitempty"`
}
