package pages

import "fmt"

// VendorVariantFormInput holds the submitted values for vendor variant creation.
type VendorVariantFormInput struct {
	ProductID              int64
	ProductName            string
	NameAR                 string
	NameEN                 string
	SKU                    string
	Barcode                string
	Unit                   string
	Price                  string
	Discount               string
	CostPrice              string
	CostDiscountPercentage string
	StockQty               string
	MinOrderQty            string
	QuotaLimit             string
	BranchID               int64
	BatchNumber            string
	ExpiryDate             string
	IsNegotiable           bool
}

// ProductIDStr returns the string representation of ProductID or empty if <= 0.
func (f VendorVariantFormInput) ProductIDStr() string {
	if f.ProductID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", f.ProductID)
}

// VendorVariantFormErrors holds per-field and general validation errors.
type VendorVariantFormErrors struct {
	General string
	Fields  map[string]string
}

// Has checks if a specific field has a validation error.
func (e VendorVariantFormErrors) Has(field string) bool {
	if e.Fields == nil {
		return false
	}
	_, ok := e.Fields[field]
	return ok
}

// Get returns the validation error message for a field or empty string.
func (e VendorVariantFormErrors) Get(field string) string {
	if e.Fields == nil {
		return ""
	}
	return e.Fields[field]
}

// VendorCustomVariantModalData provides data for rendering the add custom variant form.
type VendorCustomVariantModalData struct {
	Branches []VendorBranchOption
	Form     VendorVariantFormInput
	Errors   VendorVariantFormErrors
}
