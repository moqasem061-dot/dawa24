package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestValidGTIN(t *testing.T) {
	if !productmatch.ValidGTIN("6223000550014") {
		t.Errorf("expected 6223000550014 to be valid GTIN")
	}
	if productmatch.ValidGTIN("6223000550019") {
		t.Errorf("expected 6223000550019 to be invalid GTIN (bad check digit)")
	}
	if productmatch.ValidGTIN("12345") {
		t.Errorf("expected 12345 to be invalid GTIN (bad length)")
	}
}

func TestSuspectIdentifier(t *testing.T) {
	// Scientific notation
	reason, bad := productmatch.SuspectIdentifier("6.22E+12", 13)
	if !bad || reason != productmatch.ReasonIdentifierInvalid {
		t.Errorf("expected suspect identifier for 6.22E+12")
	}

	// Decimal point
	reason, bad = productmatch.SuspectIdentifier("622300055001.0", 13)
	if !bad || reason != productmatch.ReasonIdentifierInvalid {
		t.Errorf("expected suspect identifier for decimal point")
	}

	// Shorter than modal length (truncated leading zeroes)
	reason, bad = productmatch.SuspectIdentifier("12345", 13)
	if !bad || reason != productmatch.ReasonIdentifierInvalid {
		t.Errorf("expected suspect identifier for truncated length")
	}

	// Normal valid identifier matching modal length
	reason, bad = productmatch.SuspectIdentifier("6223000550014", 13)
	if bad {
		t.Errorf("expected normal identifier not to be suspect, got %v", reason)
	}
}

func TestDeclaredIdentifiers(t *testing.T) {
	mapping := &productmatch.Mapping{
		ByField: map[productmatch.Field]int{
			productmatch.FieldBarcode:   0,
			productmatch.FieldSKU:       1,
			productmatch.FieldProductID: 2,
			productmatch.FieldName:      3,
		},
	}

	declared := mapping.DeclaredIdentifiers()
	if len(declared) != 3 {
		t.Fatalf("expected 3 declared identifier fields, got %d", len(declared))
	}
	if declared[productmatch.FieldBarcode] != productmatch.NamespaceGTIN {
		t.Errorf("expected barcode -> gtin")
	}
	if declared[productmatch.FieldSKU] != productmatch.NamespaceSupplierCode {
		t.Errorf("expected sku -> supplier_code")
	}
	if declared[productmatch.FieldProductID] != productmatch.NamespaceCatalogCode {
		t.Errorf("expected product_id -> catalog_code")
	}
}

func TestNamespaces(t *testing.T) {
	supplierNS := productmatch.SupplierCodeNamespace("org_456")
	if supplierNS != "supplier_code:org_456" {
		t.Errorf("expected supplier_code:org_456, got %q", supplierNS)
	}

	kind, orgID, ok := productmatch.ParseNamespace(supplierNS)
	if !ok || kind != string(productmatch.NamespaceSupplierCode) || orgID != "org_456" {
		t.Errorf("ParseNamespace(%q) = (%q, %q, %v)", supplierNS, kind, orgID, ok)
	}

	kind, orgID, ok = productmatch.ParseNamespace(string(productmatch.NamespaceCatalogCode))
	if !ok || kind != string(productmatch.NamespaceCatalogCode) || orgID != "" {
		t.Errorf("ParseNamespace(catalog_code) = (%q, %q, %v)", kind, orgID, ok)
	}

	kind, orgID, ok = productmatch.ParseNamespace(string(productmatch.NamespaceGTIN))
	if !ok || kind != string(productmatch.NamespaceGTIN) || orgID != "" {
		t.Errorf("ParseNamespace(gtin) = (%q, %q, %v)", kind, orgID, ok)
	}
}

func TestValidateIdentifier(t *testing.T) {
	// Valid GTIN
	_, valid, _ := productmatch.ValidateIdentifier(string(productmatch.NamespaceGTIN), "6223000550014")
	if !valid {
		t.Errorf("expected valid GTIN identifier")
	}

	// Valid supplier code with preserved leading zeroes
	val, valid, _ := productmatch.ValidateIdentifier("supplier_code:org_1", "001234")
	if !valid || val != "001234" {
		t.Errorf("expected valid supplier code, got val=%q, valid=%v", val, valid)
	}

	// Valid catalog code
	val, valid, _ = productmatch.ValidateIdentifier(string(productmatch.NamespaceCatalogCode), "CAT-9901")
	if !valid || val != "CAT-9901" {
		t.Errorf("expected valid catalog code, got val=%q, valid=%v", val, valid)
	}
}
