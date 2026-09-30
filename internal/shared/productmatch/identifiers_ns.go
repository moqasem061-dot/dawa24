package productmatch

import (
	"fmt"
	"strings"
	"unicode"
)

// Namespace identifies the domain authority of an identifier.
type Namespace string

const (
	NamespaceGTIN         Namespace = "gtin"
	NamespaceSupplierCode Namespace = "supplier_code"
	NamespaceCatalogCode  Namespace = "catalog_code"
	PrefixSupplierCode              = "supplier_code:"
)

// ValidGTIN reports whether s is a valid GTIN (lengths 8/12/13/14 and GS1 mod-10).
func ValidGTIN(s string) bool {
	_, valid, _ := ValidateGTIN(s)
	return valid
}

// SuspectIdentifier detects malformed identifiers: scientific notation (E+), decimal points, or truncated length.
func SuspectIdentifier(s string, modalLen int) (reason ReasonCode, bad bool) {
	val := strings.TrimSpace(s)
	if val == "" {
		return ReasonIdentifierInvalid, true
	}
	if hasScientificNotation(val) || strings.Contains(val, ".") {
		return ReasonIdentifierInvalid, true
	}
	if modalLen > 0 && len(val) < modalLen {
		return ReasonIdentifierInvalid, true
	}
	return "", false
}

// DeclaredIdentifiers returns the declared identifier namespace per identifier column in the mapping.
func (m *Mapping) DeclaredIdentifiers() map[Field]Namespace {
	if m == nil || m.ByField == nil {
		return nil
	}
	out := make(map[Field]Namespace)
	if m.Has(FieldBarcode) {
		out[FieldBarcode] = NamespaceGTIN
	}
	if m.Has(FieldSKU) {
		out[FieldSKU] = NamespaceSupplierCode
	}
	if m.Has(FieldProductID) {
		out[FieldProductID] = NamespaceCatalogCode
	}
	return out
}

// SupplierCodeNamespace builds a scoped supplier code namespace.
func SupplierCodeNamespace(sourceOrgID string) string {
	return PrefixSupplierCode + strings.TrimSpace(sourceOrgID)
}

// ParseNamespace decomposes a namespace into its kind and optional org scope.
func ParseNamespace(ns string) (kind string, orgID string, ok bool) {
	ns = strings.TrimSpace(ns)
	if ns == string(NamespaceGTIN) {
		return string(NamespaceGTIN), "", true
	}
	if ns == string(NamespaceCatalogCode) {
		return string(NamespaceCatalogCode), "", true
	}
	if strings.HasPrefix(ns, PrefixSupplierCode) {
		org := strings.TrimPrefix(ns, PrefixSupplierCode)
		if org != "" {
			return string(NamespaceSupplierCode), org, true
		}
	}
	return "", "", false
}

// ValidateGTIN validates a GTIN against GS1 specifications:
// - Digits only
// - Length must be 8, 12, 13, or 14
// - Valid GS1 mod-10 check digit
// - Detects and rejects scientific notation (e.g. 6.22E+12)
func ValidateGTIN(raw string) (string, bool, string) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return "", false, "empty_identifier"
	}

	if hasScientificNotation(val) {
		return val, false, "scientific_notation"
	}

	for _, r := range val {
		if !unicode.IsDigit(r) {
			return val, false, "non_numeric"
		}
	}

	n := len(val)
	if n != 8 && n != 12 && n != 13 && n != 14 {
		return val, false, "invalid_length"
	}

	if !isValidGS1Mod10(val) {
		return val, false, "checksum_error"
	}

	return val, true, ""
}

// ValidateIdentifier validates an identifier value within its declared namespace.
func ValidateIdentifier(namespace, value string) (string, bool, string) {
	val := strings.TrimSpace(value)
	if val == "" {
		return "", false, "empty_identifier"
	}

	kind, _, ok := ParseNamespace(namespace)
	if !ok {
		return val, false, "unrecognized_namespace"
	}

	if hasScientificNotation(val) {
		return val, false, "scientific_notation"
	}

	switch kind {
	case string(NamespaceGTIN):
		return ValidateGTIN(val)
	case string(NamespaceCatalogCode), string(NamespaceSupplierCode):
		if len(val) == 0 {
			return val, false, "empty_code"
		}
		return val, true, ""
	default:
		return val, false, fmt.Sprintf("unsupported_kind_%s", kind)
	}
}

func hasScientificNotation(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "e+") || strings.Contains(lower, "e-")
}

func isValidGS1Mod10(s string) bool {
	n := len(s)
	if n < 2 {
		return false
	}

	sum := 0
	weight := 3

	for i := n - 2; i >= 0; i-- {
		d := int(s[i] - '0')
		sum += d * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}

	expectedCheck := (10 - (sum % 10)) % 10
	actualCheck := int(s[n-1] - '0')
	return actualCheck == expectedCheck
}
