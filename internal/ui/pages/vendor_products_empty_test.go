package pages_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorProducts_EmptyStateAddFromCatalogButton(t *testing.T) {
	data := pages.VendorVariantsData{
		Variants: []*pages.VendorVariantView{},
	}

	var buf bytes.Buffer
	err := pages.VendorProducts(data, "ar", "rtl", false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	html := buf.String()

	// 1. Must contain the empty state title and message
	if !strings.Contains(html, "لا توجد أصناف توريد مسجلة حالياً") {
		t.Errorf("expected empty state title in rendered HTML")
	}

	// 2. Must contain the button with data-modal-open and action handler
	if !strings.Contains(html, `data-modal-open="add-from-catalog-modal"`) {
		t.Errorf("expected empty state button to have data-modal-open='add-from-catalog-modal'")
	}
	if !strings.Contains(html, `data-on-click="openAddFromCatalogModal()"`) && !strings.Contains(html, `onclick="openAddFromCatalogModal()"`) {
		t.Errorf("expected empty state button to have data-on-click or onclick")
	}

	// 3. Must NOT contain broken javascript: pseudo-protocols
	if strings.Contains(html, `href="javascript:`) {
		t.Errorf("expected NO href='javascript:...' in rendered HTML")
	}

	// 4. Must be a <button type="button"> rather than an unclickable or blocked <a> tag
	if !strings.Contains(html, `<button type="button" data-modal-open="add-from-catalog-modal"`) {
		t.Errorf("expected valid <button type='button'> tag for empty state action")
	}
}
