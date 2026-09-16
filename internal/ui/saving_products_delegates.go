package ui

import (
	"net/http"
)

// Customer Saving Products Handlers (public delegates)

// CustomerSavingProductsPage renders the customer's live price-delta tracking list.
func (h *UIHandler) CustomerSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPage(w, r, "customer")
}

// CustomerSavingProductCreateSubmit handles manual creation of a pharmacy saving product.
func (h *UIHandler) CustomerSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductCreateSubmit(w, r, "customer")
}

// CustomerSavingProductUpdateSubmit handles updating an existing pharmacy saving product.
func (h *UIHandler) CustomerSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductUpdateSubmit(w, r, "customer")
}

// CustomerSavingProductDeleteSubmit deletes a saving product record for the pharmacy.
func (h *UIHandler) CustomerSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteSubmit(w, r, "customer")
}

// CustomerSavingProductsBulkDeleteSubmit deletes multiple selected saving products for the pharmacy.
func (h *UIHandler) CustomerSavingProductsBulkDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsBulkDeleteSubmit(w, r, "customer")
}

// CustomerSavingProductsDeleteAllSubmit deletes all saving products for the customer org.
func (h *UIHandler) CustomerSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteAllSubmit(w, r, "customer")
}

// CustomerSavingProductsExport streams an Excel spreadsheet of all saving products for the pharmacy.
func (h *UIHandler) CustomerSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsExport(w, r, "customer")
}

// CustomerSavingProductProvidersJSON delegates to SavingProductProvidersJSON logic.
func (h *UIHandler) CustomerSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductProvidersJSON(w, r)
}

// CustomerSavingProductSearchJSON delegates to SavingProductSearchJSON logic.
func (h *UIHandler) CustomerSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductSearchJSON(w, r)
}

// CustomerSavingProductDetailPage renders single saving product delta details.
func (h *UIHandler) CustomerSavingProductDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusSeeOther)
}

// CustomerSavingProductsAlias redirects misspelled route /customer/saveing-products.
func (h *UIHandler) CustomerSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusMovedPermanently)
}

// Vendor Saving Products Handlers (public delegates)

// VendorSavingProductsPage renders the vendor's saving products directory.
func (h *UIHandler) VendorSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPage(w, r, "vendor")
}

// VendorSavingProductCreateSubmit handles manual creation of a saving product for vendor.
func (h *UIHandler) VendorSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductCreateSubmit(w, r, "vendor")
}

// VendorSavingProductUpdateSubmit handles updating an existing saving product for vendor.
func (h *UIHandler) VendorSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductUpdateSubmit(w, r, "vendor")
}

// VendorSavingProductDeleteSubmit deletes a saving product record for vendor.
func (h *UIHandler) VendorSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteSubmit(w, r, "vendor")
}

// VendorSavingProductsBulkDeleteSubmit deletes multiple selected saving products for vendor.
func (h *UIHandler) VendorSavingProductsBulkDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsBulkDeleteSubmit(w, r, "vendor")
}

// VendorSavingProductsDeleteAllSubmit deletes all saving products for the vendor org.
func (h *UIHandler) VendorSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteAllSubmit(w, r, "vendor")
}

// VendorSavingProductsExport streams an Excel spreadsheet of all saving products for vendor.
func (h *UIHandler) VendorSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsExport(w, r, "vendor")
}

// VendorSavingProductProvidersJSON delegates to SavingProductProvidersJSON logic.
func (h *UIHandler) VendorSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductProvidersJSON(w, r)
}

// VendorSavingProductSearchJSON delegates to SavingProductSearchJSON logic.
func (h *UIHandler) VendorSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductSearchJSON(w, r)
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
