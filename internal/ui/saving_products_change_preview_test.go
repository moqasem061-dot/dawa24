package ui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

type mockExistingSavingRepo struct {
	*mockCatalogMatchingRepo
	existingProducts []*catalog.SavingProduct
}

func (m *mockExistingSavingRepo) ListSavingProductsByOrg(_ context.Context, orgID int64, _, _ int) ([]*catalog.SavingProduct, error) {
	var res []*catalog.SavingProduct
	for _, p := range m.existingProducts {
		if p.OrganizationID == orgID {
			res = append(res, p)
		}
	}
	return res, nil
}

func TestDetectPackUnit(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"بانادول إكسترا 24 قرص", "قرص"},
		{"كونجستال 20 كبسولة", "كبسولة"},
		{"أوجمنتين 1 جم حقن أمبول", "أمبول"},
		{"فيتامين سي فوار", "كيس فوار"},
		{"قطرة توبريكس نقط للعين", "قطرة"},
		{"ميبو مرهم", "أنبوبة مرهم"},
		{"بنزيل بنزوات كريم", "أنبوبة كريم"},
		{"عقار غير محدد", "علبة"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectPackUnit(tt.name))
		})
	}
}

func TestAttachSavingChangePreviews(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const orgID int64 = 77

	oldPrice, _ := money.Parse("20.00")
	newPrice, _ := money.Parse("25.50")
	pid := int64(101)

	mockRepo := &mockExistingSavingRepo{
		mockCatalogMatchingRepo: &mockCatalogMatchingRepo{
			mockCatalogImageRepo: newMockCatalogImageRepo(),
		},
		existingProducts: []*catalog.SavingProduct{
			{
				ID:             1,
				OrganizationID: orgID,
				ProductID:      &pid,
				NameProduct:    "بانادول إكسترا 24 قرص",
				SKU:            "PAN-EXT",
				Quantity:       5,
				Price:          oldPrice,
			},
		},
	}

	catSvc := catalog.NewService(mockRepo, logger)
	h := NewUIHandler(catSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	stagedItems := []*StagedSavingItem{
		{
			Index:             0,
			NameProduct:       "بانادول إكسترا 24 قرص",
			SKU:               "PAN-EXT",
			Quantity:          10,
			Price:             newPrice,
			TotalValue:        money.FromMinor(25500),
			ProductID:         &pid,
			MasterProductName: "بانادول إكسترا 24 قرص",
			MasterProductSKU:  "PAN-EXT",
			Included:          true,
		},
		{
			Index:             1,
			NameProduct:       "كونجستال 20 قرص جديد",
			SKU:               "CONG-NEW",
			Quantity:          2,
			Price:             newPrice,
			TotalValue:        money.FromMinor(5100),
			Included:          true,
		},
	}

	h.attachSavingChangePreviews(context.Background(), orgID, stagedItems)

	// Item 0 matches existing saving product
	item0 := stagedItems[0]
	require.NotNil(t, item0.ChangePreview)
	assert.True(t, item0.ChangePreview.IsUpdate)
	assert.Equal(t, int64(1), item0.ChangePreview.ExistingID)
	assert.Equal(t, float64(5), item0.ChangePreview.OldQuantity)
	assert.Equal(t, float64(10), item0.ChangePreview.NewQuantity)
	assert.Equal(t, "20.00", item0.ChangePreview.OldPrice.String())
	assert.Equal(t, "25.50", item0.ChangePreview.NewPrice.String())
	assert.Equal(t, "قرص", item0.ChangePreview.OldPackUnit)
	assert.Equal(t, "قرص", item0.ChangePreview.NewPackUnit)

	// Item 1 is brand new, no existing entry
	item1 := stagedItems[1]
	assert.Nil(t, item1.ChangePreview)

	// Test template rendering with pre-commit change preview
	view := pages.SavingImportView{
		Audience:  "pharmacy",
		ImportURL: "/customer/saving-products/import",
		Session: &pages.SavingImportSession{
			ID:    "sess-preview-test",
			OrgID: orgID,
			Phase: pages.SavingPhaseReview,
		},
		Rows: stagedItems,
	}

	var buf bytes.Buffer
	err := pages.SavingImportPage(view, "ar", "rtl").Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// Assert pre-commit preview badge and old/new values are present
	assert.True(t, strings.Contains(html, "تعديل صنف حالي"), "expected update badge in HTML")
	assert.True(t, strings.Contains(html, "20.00"), "expected old price in HTML")
	assert.True(t, strings.Contains(html, "25.50"), "expected new price in HTML")
	assert.True(t, strings.Contains(html, "العبوة: قرص ← قرص"), "expected pack unit change preview in HTML")
}
