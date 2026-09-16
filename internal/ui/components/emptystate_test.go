package components_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/components"
)

func TestEmptyState_LinkAction(t *testing.T) {
	var buf bytes.Buffer
	props := components.EmptyStateProps{
		Title:       "لا توجد نتائج",
		Message:     "يرجى تجربة معايير بحث أخرى",
		ActionLabel: "تصفح الكتالوج",
		ActionURL:   "/catalog",
	}
	err := components.EmptyState(props).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<a") || !strings.Contains(html, `href="/catalog"`) {
		t.Errorf("expected <a> link with href='/catalog', got: %s", html)
	}
	if !strings.Contains(html, "تصفح الكتالوج") {
		t.Errorf("expected ActionLabel in rendered output, got: %s", html)
	}
}

func TestEmptyState_ModalAndOnClickButton(t *testing.T) {
	var buf bytes.Buffer
	props := components.EmptyStateProps{
		Title:         "لا توجد أصناف",
		Message:       "قم باختيار الأدوية",
		ActionLabel:   "+ إضافة صنف من الكتالوج العام",
		ActionModalID: "add-from-catalog-modal",
		ActionOnClick: "openAddFromCatalogModal()",
	}
	err := components.EmptyState(props).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<button") || !strings.Contains(html, `type="button"`) {
		t.Errorf("expected <button type='button'>, got: %s", html)
	}
	if !strings.Contains(html, `data-modal-open="add-from-catalog-modal"`) {
		t.Errorf("expected data-modal-open attribute, got: %s", html)
	}
	if !strings.Contains(html, `data-on-click="openAddFromCatalogModal()"`) {
		t.Errorf("expected data-on-click attribute, got: %s", html)
	}
	if strings.Contains(html, "javascript:") {
		t.Errorf("unexpected javascript: pseudo-protocol in button: %s", html)
	}
}

func TestEmptyState_ConvertsLegacyJavascriptActionURLToButton(t *testing.T) {
	var buf bytes.Buffer
	props := components.EmptyStateProps{
		Title:       "لا توجد أصناف",
		Message:     "قم باختيار الأدوية",
		ActionLabel: "+ إضافة صنف من الكتالوج العام",
		ActionURL:   "javascript:openAddFromCatalogModal()",
	}
	err := components.EmptyState(props).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<button") || !strings.Contains(html, `type="button"`) {
		t.Errorf("expected <button type='button'> for javascript: ActionURL, got: %s", html)
	}
	if !strings.Contains(html, `data-on-click="openAddFromCatalogModal()"`) {
		t.Errorf("expected data-on-click='openAddFromCatalogModal()', got: %s", html)
	}
	if strings.Contains(html, "<a") || strings.Contains(html, "href=") {
		t.Errorf("expected NO <a> or href attribute for javascript: action, got: %s", html)
	}
}
