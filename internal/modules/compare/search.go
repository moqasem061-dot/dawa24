package compare

import (
	"context"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CompareSearchResultItem represents an aggregated product match across supplier sheets and master catalog.
type CompareSearchResultItem struct {
	ProductName          string                   `json:"product_name"`
	SKU                  string                   `json:"sku,omitempty"`
	CatalogProductID     *int64                   `json:"catalog_product_id,omitempty"`
	InCatalog            bool                     `json:"in_catalog"`
	CatalogStatus        CatalogStatus            `json:"catalog_status"` // catalog_and_suppliers | catalog_only | supplier_custom
	BestPrice            money.Amount             `json:"best_price"`
	BestDiscount         float64                  `json:"best_discount"`
	BestNetPrice         money.Amount             `json:"best_net_price"`
	BestSupplier         string                   `json:"best_supplier"`
	Offers               map[string]SupplierOffer `json:"offers"`
	AvailableSuppliers   []string                 `json:"available_suppliers"`
	MissingFromSuppliers []string                 `json:"missing_from_suppliers"`
	TotalSuppliers       int                      `json:"total_suppliers"`
}

// CompareSearchResults is the top-level payload for the quick search engine.
type CompareSearchResults struct {
	Query            string                     `json:"query"`
	TotalMatches     int                        `json:"total_matches"`
	InCatalogCount   int                        `json:"in_catalog_count"`
	CustomItemsCount int                        `json:"custom_items_count"`
	Items            []*CompareSearchResultItem `json:"items"`
}

// SearchAcrossSuppliersAndCatalog searches across user price lists and the platform's master catalog,
// strictly differentiating between (1) Catalog verified products, (2) Active supplier listings, and (3) Supplier gaps.
func (s *Service) SearchAcrossSuppliersAndCatalog(ctx context.Context, userID int64, orgID *int64, query string) (*CompareSearchResults, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return &CompareSearchResults{
			Query: query,
			Items: []*CompareSearchResultItem{},
		}, nil
	}

	// 1. Fetch user's active compare files to determine all currently active suppliers
	activeStatus := FileReady
	files, err := s.repo.ListFiles(ctx, userID, orgID, &activeStatus)
	if err != nil {
		s.log.WarnContext(ctx, "list active compare files warning", "error", err)
	}

	var allActiveSuppliers []string
	supplierSet := make(map[string]bool)
	for _, f := range files {
		if !supplierSet[f.SupplierName] {
			supplierSet[f.SupplierName] = true
			allActiveSuppliers = append(allActiveSuppliers, f.SupplierName)
		}
	}

	// 2. Search user's uploaded supplier rows
	fileRows, err := s.repo.SearchFileRows(ctx, userID, orgID, query, 100)
	if err != nil {
		s.log.WarnContext(ctx, "search supplier file rows warning", "error", err, "query", query)
	}

	// 3. Search master catalog products
	catalogCandidates, err := s.repo.FindCandidateProducts(ctx, orgID, query, query, 30)
	if err != nil {
		s.log.WarnContext(ctx, "search catalog candidates warning", "error", err, "query", query)
	}

	// 4. Index structures for correlating products
	var items []*CompareSearchResultItem
	byCatalogID := make(map[int64]*CompareSearchResultItem)
	bySKU := make(map[string]*CompareSearchResultItem)
	byNorm := make(map[string]*CompareSearchResultItem)
	bySorted := make(map[string]*CompareSearchResultItem)

	// Pre-seed catalog products
	for _, c := range catalogCandidates {
		name := c.NameAr
		if name == "" {
			name = c.NameEn
		}
		if name == "" {
			name = c.ScientificName
		}
		if name == "" {
			continue
		}

		cleanSKU := strings.ToLower(strings.TrimSpace(c.SKU))
		norm := normalizeProductText(name)
		sortedKey := getSortedWordsKey(name)

		prodID := c.ID
		item := &CompareSearchResultItem{
			ProductName:      name,
			SKU:              c.SKU,
			CatalogProductID: &prodID,
			InCatalog:        true,
			CatalogStatus:    StatusCatalogOnly,
			Offers:           make(map[string]SupplierOffer),
		}

		items = append(items, item)
		byCatalogID[c.ID] = item
		if cleanSKU != "" {
			bySKU[cleanSKU] = item
		}
		if norm != "" {
			byNorm[norm] = item
		}
		if sortedKey != "" {
			bySorted[sortedKey] = item
		}
	}

	// Correlate supplier file rows
	for _, r := range fileRows {
		cleanSKU := strings.ToLower(strings.TrimSpace(r.SKU))
		norm := normalizeProductText(r.RawName)
		if norm == "" {
			norm = r.NormalizedName
		}
		sortedKey := getSortedWordsKey(r.RawName)

		var targetItem *CompareSearchResultItem

		// Match ladder: Catalog ID -> SKU -> Exact Normalized -> Sorted Words
		if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
			targetItem = byCatalogID[*r.MatchedProductID]
		}
		if targetItem == nil && cleanSKU != "" {
			targetItem = bySKU[cleanSKU]
		}
		if targetItem == nil && norm != "" {
			targetItem = byNorm[norm]
		}
		if targetItem == nil && sortedKey != "" {
			targetItem = bySorted[sortedKey]
		}

		netPrice := r.PriceAfterDiscount
		if netPrice.IsZero() && r.Price.IsPositive() {
			netPrice = CalculatePriceAfterDiscount(r.Price, r.Discount)
		}

		offer := SupplierOffer{
			SupplierName:       r.SupplierName,
			Price:              r.Price,
			Discount:           r.Discount,
			PriceAfterDiscount: netPrice,
		}

		if targetItem == nil {
			inCatalog := (r.MatchedProductID != nil && *r.MatchedProductID > 0)
			status := StatusSupplierCustom
			if inCatalog {
				status = StatusCatalogAndSuppliers
			}
			targetItem = &CompareSearchResultItem{
				ProductName:      r.RawName,
				SKU:              r.SKU,
				CatalogProductID: r.MatchedProductID,
				InCatalog:        inCatalog,
				CatalogStatus:    status,
				Offers:           make(map[string]SupplierOffer),
				BestPrice:        r.Price,
				BestDiscount:     r.Discount,
				BestNetPrice:     netPrice,
				BestSupplier:     r.SupplierName,
			}
			items = append(items, targetItem)

			if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
				byCatalogID[*r.MatchedProductID] = targetItem
			}
			if cleanSKU != "" {
				bySKU[cleanSKU] = targetItem
			}
			if norm != "" {
				byNorm[norm] = targetItem
			}
			if sortedKey != "" {
				bySorted[sortedKey] = targetItem
			}
		} else if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
			targetItem.InCatalog = true
			targetItem.CatalogProductID = r.MatchedProductID
		}

		targetItem.Offers[r.SupplierName] = offer

		// Update best pricing
		if targetItem.BestNetPrice.IsZero() || netPrice.Minor() < targetItem.BestNetPrice.Minor() || (netPrice.Minor() == targetItem.BestNetPrice.Minor() && r.Discount > targetItem.BestDiscount) {
			targetItem.BestNetPrice = netPrice
			targetItem.BestPrice = r.Price
			targetItem.BestDiscount = r.Discount
			targetItem.BestSupplier = r.SupplierName
		}
	}

	// 5. Finalize status, supplier lists, and gaps for every item
	var finalItems []*CompareSearchResultItem
	inCatalogCount := 0
	customCount := 0

	for _, it := range items {
		it.TotalSuppliers = len(it.Offers)

		// Determine 3-way status
		if len(it.Offers) > 0 {
			if it.InCatalog {
				it.CatalogStatus = StatusCatalogAndSuppliers
				inCatalogCount++
			} else {
				it.CatalogStatus = StatusSupplierCustom
				customCount++
			}
		} else {
			it.CatalogStatus = StatusCatalogOnly
			inCatalogCount++
		}

		// Calculate available and missing suppliers
		for sup := range it.Offers {
			it.AvailableSuppliers = append(it.AvailableSuppliers, sup)
		}
		sort.Strings(it.AvailableSuppliers)

		for _, sup := range allActiveSuppliers {
			if _, exists := it.Offers[sup]; !exists {
				it.MissingFromSuppliers = append(it.MissingFromSuppliers, sup)
			}
		}

		finalItems = append(finalItems, it)
	}

	// Sort results: Available items first (by best net price), then catalog items
	sort.Slice(finalItems, func(i, j int) bool {
		iHasOffers := len(finalItems[i].Offers) > 0
		jHasOffers := len(finalItems[j].Offers) > 0
		if iHasOffers != jHasOffers {
			return iHasOffers
		}
		if iHasOffers && jHasOffers {
			if finalItems[i].TotalSuppliers != finalItems[j].TotalSuppliers {
				return finalItems[i].TotalSuppliers > finalItems[j].TotalSuppliers
			}
			return finalItems[i].BestNetPrice.Minor() < finalItems[j].BestNetPrice.Minor()
		}
		return finalItems[i].ProductName < finalItems[j].ProductName
	})

	return &CompareSearchResults{
		Query:            query,
		TotalMatches:     len(finalItems),
		InCatalogCount:   inCatalogCount,
		CustomItemsCount: customCount,
		Items:            finalItems,
	}, nil
}
