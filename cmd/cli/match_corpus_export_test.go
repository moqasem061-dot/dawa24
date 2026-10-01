package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildDeidentifiedCase_StripsPrivateFieldsAndHashesCodes(t *testing.T) {
	orgID := int64(42)
	rowID := int64(1001)
	parentID := int64(500)
	rawName := "بانادول اكسترا 500 مجم"
	supplierCode := "SUPP-98765"
	productID := int64(48213)

	payload := map[string]any{
		"name":            rawName,
		"price":           "60.00",
		"cost":            "45.00",
		"qty":             10,
		"quantity":        25,
		"organization_id": 42,
		"org_name":        "Private Pharmacy",
		"strength":        "500mg",
		"form":            "tablet",
	}
	payloadBytes, _ := json.Marshal(payload)

	c := buildDeidentifiedCase(
		"vendor",
		rowID,
		parentID,
		orgID,
		rawName,
		supplierCode,
		&productID,
		payloadBytes,
		"invoice_october.xlsx",
	)

	// 1. Case ID format
	assert.Equal(t, "c-vendor-1001", c.ID)

	// 2. Private fields must be completely stripped
	for k := range c.Row.Cells {
		assert.False(t, strings.Contains(k, "org"), "key %q leaked in cells", k)
		assert.False(t, strings.Contains(k, "price"), "key %q leaked in cells", k)
		assert.False(t, strings.Contains(k, "cost"), "key %q leaked in cells", k)
		assert.False(t, strings.Contains(k, "qty"), "key %q leaked in cells", k)
		assert.False(t, strings.Contains(k, "quantity"), "key %q leaked in cells", k)
	}

	// 3. Supplier code replaced with namespace-preserving surrogate hash
	assert.Len(t, c.Row.Identifiers, 1)
	assert.Equal(t, "code", c.Row.Identifiers[0].Column)
	assert.Equal(t, "supplier_code", c.Row.Identifiers[0].Namespace)
	assert.True(t, strings.HasPrefix(c.Row.Cells["code"], "sc_"))
	assert.NotContains(t, c.Row.Cells["code"], "SUPP-98765", "original supplier code leaked")

	// 4. Family identifiers generated
	assert.True(t, strings.HasPrefix(c.TemplateFamily, "tf-"))
	assert.True(t, strings.HasPrefix(c.ExpressionFamily, "ef-"))

	// 5. Label and resolvable target
	assert.True(t, c.Label.Resolvable)
	assert.Equal(t, []int64{48213}, c.Label.Targets)
	assert.Equal(t, "accept", c.Label.Expect)
	assert.Equal(t, "ar", c.Language)
}

func TestDetectCaseLanguage(t *testing.T) {
	assert.Equal(t, "ar", detectCaseLanguage("بانادول اكسترا"))
	assert.Equal(t, "en", detectCaseLanguage("Panadol Extra 500mg"))
	assert.Equal(t, "mixed", detectCaseLanguage("بانادول Panadol Extra"))
}
