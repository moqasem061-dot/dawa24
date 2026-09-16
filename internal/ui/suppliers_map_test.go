package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestSuppliersMap_PinsRenderingAndInteraction(t *testing.T) {
	cairoAddr := "شارع جامع عابدين، القاهرة"
	aswanAddr := "شارع خلف مسجد الطابيه، أسوان"
	riyadhAddr := "شارع العليا، riyadh"

	branchCairo := &org.Branch{
		ID:       101,
		Name:     i18n.Text{i18n.AR: "فرع القاهرة", i18n.EN: "Cairo Branch"},
		Address:  cairoAddr,
		IsMain:   true,
		Status:   "active",
	}
	branchAswan := &org.Branch{
		ID:       102,
		Name:     i18n.Text{i18n.AR: "فرع أسوان", i18n.EN: "Aswan Branch"},
		Address:  aswanAddr,
		IsMain:   false,
		Status:   "active",
	}
	branchRiyadh := &org.Branch{
		ID:       103,
		Name:     i18n.Text{i18n.AR: "فرع الرياض", i18n.EN: "Riyadh Branch"},
		Address:  riyadhAddr,
		IsMain:   false,
		Status:   "active",
	}

	supplier := &pages.SupplierDirectoryItem{
		Org: &org.Organization{
			ID:        55,
			TradeName: i18n.Text{i18n.AR: "مورد التجربة", i18n.EN: "Test Supplier"},
			LegalName: "Test Supplier LLC",
			Status:    org.StatusApproved,
		},
		Branches: []*org.Branch{branchCairo, branchAswan, branchRiyadh},
	}

	pins := pages.BuildSuppliersMapItems([]*pages.SupplierDirectoryItem{supplier}, "ar")
	if len(pins) != 3 {
		t.Fatalf("expected 3 branch pins, got %d", len(pins))
	}

	// Verify city detection
	aswanPin := pins[1]
	if aswanPin.Lat < 23 || aswanPin.Lat > 25 {
		t.Errorf("expected Aswan latitude around 24, got %f", aswanPin.Lat)
	}
	if aswanPin.Lng < 31 || aswanPin.Lng > 34 {
		t.Errorf("expected Aswan longitude around 32-33, got %f", aswanPin.Lng)
	}

	riyadhPin := pins[2]
	if riyadhPin.Lat < 23 || riyadhPin.Lat > 26 {
		t.Errorf("expected Riyadh latitude around 24-25, got %f", riyadhPin.Lat)
	}
	if riyadhPin.Lng < 45 || riyadhPin.Lng > 48 {
		t.Errorf("expected Riyadh longitude around 46-47, got %f", riyadhPin.Lng)
	}

	data := pages.SupplierDirectoryData{
		ActiveTab: "map",
		AllPins:   pins,
		Suppliers: []*pages.SupplierDirectoryItem{supplier},
	}

	var buf bytes.Buffer
	err := pages.SuppliersMap("ar", "rtl", data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := buf.String()

	// 1. Must NOT contain unparsed Go code
	if strings.Contains(out, "{ templ.Raw(") || strings.Contains(out, "if len(data.AllPins)") {
		t.Errorf("found raw unparsed Go code in SuppliersMap HTML")
	}

	// 2. Must contain #suppliers-map-data with valid JSON
	re := regexp.MustCompile(`id="suppliers-map-data"[^>]*data-pins="([^"]+)"`)
	matches := re.FindStringSubmatch(out)
	if len(matches) < 2 {
		t.Fatalf("could not find #suppliers-map-data with data-pins in output: %s", out)
	}

	rawJSON := html.UnescapeString(matches[1])
	var parsedPins []pages.SuppliersMapItem
	if err := json.Unmarshal([]byte(rawJSON), &parsedPins); err != nil {
		t.Fatalf("failed to parse JSON from data-pins: %v\nraw: %s", err, rawJSON)
	}
	if len(parsedPins) != 3 {
		t.Errorf("expected 3 parsed pins in JSON, got %d", len(parsedPins))
	}

	// 3. Must contain map-supplier-item with data-supplier-idx
	if !strings.Contains(out, `data-supplier-idx="0"`) || !strings.Contains(out, `data-supplier-idx="1"`) {
		t.Errorf("expected data-supplier-idx attributes on sidebar items")
	}

	// 4. Must contain click delegation and focusSupplierOnMap
	if !strings.Contains(out, "focusSupplierOnMap") {
		t.Errorf("expected focusSupplierOnMap in script")
	}
}
