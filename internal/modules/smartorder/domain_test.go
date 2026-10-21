package smartorder

import (
	"testing"
)

func TestMatchMethod_Valid(t *testing.T) {
	validMethods := []MatchMethod{
		MethodSavingProduct,
		MethodLearnedMapping,
		MethodBarcode,
		MethodSKU,
		MethodExactName,
		MethodIdentityKey,
		MethodFuzzy,
		MethodAlias,
		MethodAI,
		MethodManual,
		MethodNone,
	}

	for _, m := range validMethods {
		if !m.Valid() {
			t.Errorf("expected method %q to be valid", m)
		}
	}

	invalidMethods := []MatchMethod{
		"exact",
		"strong",
		"code",
		"invalid",
		"",
	}

	for _, m := range invalidMethods {
		if m.Valid() {
			t.Errorf("expected method %q to be invalid", m)
		}
	}
}

func TestSanitizeMatchMethod(t *testing.T) {
	tests := []struct {
		input MatchMethod
		want  MatchMethod
	}{
		{MethodBarcode, MethodBarcode},
		{MethodSKU, MethodSKU},
		{MethodExactName, MethodExactName},
		{"exact", MethodExactName},
		{"strong", MethodIdentityKey},
		{"code", MethodSKU},
		{"barcode", MethodBarcode},
		{"ai", MethodAI},
		{"fuzzy", MethodFuzzy},
		{"", MethodNone},
		{"unknown_tier", MethodNone},
	}

	for _, tt := range tests {
		got := SanitizeMatchMethod(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeMatchMethod(%q) = %q, want %q", tt.input, got, tt.want)
		}
		if !got.Valid() {
			t.Errorf("SanitizeMatchMethod(%q) produced invalid method %q", tt.input, got)
		}
	}
}
