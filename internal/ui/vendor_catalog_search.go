package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

type vendorCatalogSearchResult struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Hint           string `json:"hint,omitempty"`
	Badge          string `json:"badge,omitempty"`
	RawID          int64  `json:"raw_id"`
	NameAR         string `json:"name_ar"`
	NameEN         string `json:"name_en"`
	ScientificName string `json:"scientific_name"`
	DosageForm     string `json:"dosage_form"`
	Concentration  string `json:"concentration"`
	Unit           string `json:"unit"`
	SKU            string `json:"sku"`
	Barcode        string `json:"barcode"`
	Price          string `json:"price"`
	Manufacturer   string `json:"manufacturer"`
}

// VendorCatalogSearchJSON returns JSON autocomplete list of general catalog products.
func (h *UIHandler) VendorCatalogSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		writeVendorCatalogJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]vendorCatalogSearchResult{})
		return
	}
	if h.catSvc == nil {
		writeVendorCatalogJSONError(w, http.StatusServiceUnavailable, "catalog_unavailable")
		return
	}
	results, err := h.searchCentralCatalog(ctx, query, langOf(r))
	if err != nil {
		h.logLoadErr(ctx, "catSvc.Search", err)
		writeVendorCatalogJSONError(w, http.StatusServiceUnavailable, "catalog_search_failed")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *UIHandler) searchCentralCatalog(ctx context.Context, query, lang string) ([]vendorCatalogSearchResult, error) {
	products, err := h.catSvc.Search(ctx, catalog.SearchParams{Query: query, Limit: 20, SkipStockPriority: true})
	if err != nil {
		return nil, err
	}
	results := make([]vendorCatalogSearchResult, 0, len(products))
	for _, product := range products {
		results = append(results, vendorCatalogResult(product, lang))
	}
	return results, nil
}

func writeVendorCatalogJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func vendorCatalogResult(product *catalog.Product, lang string) vendorCatalogSearchResult {
	return vendorCatalogSearchResult{
		ID: strconv.FormatInt(product.ID, 10), Label: vendorCatalogLabel(product),
		Hint: vendorCatalogHint(product), Badge: product.Price.String() + " " + i18n.T(lang, "common.currency_egp"), RawID: product.ID,
		NameAR: product.Name.Get(i18n.AR), NameEN: product.Name.Get(i18n.EN),
		ScientificName: product.ScientificName, DosageForm: product.DosageForm,
		Concentration: product.Concentration, Unit: product.Unit,
		SKU: product.SKU, Barcode: product.Barcode, Price: product.Price.String(),
		Manufacturer: product.ManufacturingCompanies,
	}
}

func vendorCatalogLabel(product *catalog.Product) string {
	label := product.Name.Get(i18n.AR)
	if label == "" {
		label = product.Name.Get(i18n.EN)
	}
	if product.DosageForm != "" {
		label += " — " + product.DosageForm
	}
	return label
}

func vendorCatalogHint(product *catalog.Product) string {
	if product.SKU == "" {
		return product.ScientificName
	}
	if product.ScientificName == "" {
		return product.SKU
	}
	return product.ScientificName + " · " + product.SKU
}
