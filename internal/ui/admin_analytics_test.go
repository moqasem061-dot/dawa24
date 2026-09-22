package ui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestSortedStatItems(t *testing.T) {
	counts := map[string]int{
		"Alexandria": 40,
		"Cairo":      150,
		"Giza":       80,
		"Mansoura":   10,
	}

	sorted := pages.SortedStatItems(counts, 280)
	if len(sorted) != 4 {
		t.Fatalf("expected 4 items, got %d", len(sorted))
	}

	if sorted[0].Key != "Cairo" || sorted[0].Count != 150 {
		t.Errorf("expected Cairo first with 150, got %s with %d", sorted[0].Key, sorted[0].Count)
	}
	if sorted[1].Key != "Giza" || sorted[1].Count != 80 {
		t.Errorf("expected Giza second with 80, got %s with %d", sorted[1].Key, sorted[1].Count)
	}
	if sorted[2].Key != "Alexandria" || sorted[2].Count != 40 {
		t.Errorf("expected Alexandria third with 40, got %s with %d", sorted[2].Key, sorted[2].Count)
	}
	if sorted[3].Key != "Mansoura" || sorted[3].Count != 10 {
		t.Errorf("expected Mansoura fourth with 10, got %s with %d", sorted[3].Key, sorted[3].Count)
	}
}

func TestAdminAnalyticsPage_RenderWithHighLocationVolume(t *testing.T) {
	// Simulate 50 cities to ensure no panic and that the scrollable container is present
	cities := make(map[string]int, 50)
	for i := 1; i <= 50; i++ {
		cities[fmt.Sprintf("City-%02d", i)] = i * 3
	}

	analytics := &platformadmin.VisitorAnalytics{
		Total:           5000,
		Today:           250,
		TotalPharmacies: 120,
		TotalSuppliers:  15,
		TotalOrders:     430,
		TotalGMV:        "1,250,000 EGP",
		ByCountry: map[string]int{
			"مصر (Egypt)":               4800,
			"المملكة العربية السعودية": 150,
			"الإمارات":                 50,
		},
		ByCity: cities,
		ByDevice: map[string]int{
			"desktop": 3200,
			"mobile":  1700,
			"tablet":  100,
		},
		ByOS: map[string]int{
			"Windows": 2800,
			"Android": 1200,
			"iOS":     600,
			"macOS":   400,
		},
		ByBrowser: map[string]int{
			"Chrome":  3500,
			"Safari":  1000,
			"Firefox": 350,
			"Edge":    150,
		},
		Recent: []*platformadmin.Visitor{
			{
				IP:        "197.34.***.***",
				Country:   "مصر",
				City:      "القاهرة",
				Device:    "desktop",
				Browser:   "Chrome",
				OS:        "Windows",
				VisitedAt: time.Now(),
				CreatedAt: time.Now(),
			},
		},
		Page:    1,
		PerPage: 15,
	}

	var buf bytes.Buffer
	err := pages.AdminAnalytics("ar", "rtl", analytics).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render AdminAnalytics page: %v", err)
	}

	output := buf.String()
	// 1. Verify scrollable container is rendered for high location count
	if !strings.Contains(output, "max-h-[300px] overflow-y-auto") {
		t.Errorf("expected scrollable container 'max-h-[300px] overflow-y-auto' in output")
	}

	// 2. Verify metric cards are rendered
	if !strings.Contains(output, "إجمالي الزيارات") || !strings.Contains(output, "زيارات اليوم") {
		t.Errorf("expected KPI labels in rendered output")
	}

	// 3. Verify recent visit row is present
	if !strings.Contains(output, "197.34.***.***") || !strings.Contains(output, "القاهرة") {
		t.Errorf("expected visitor IP and city in rendered output")
	}
}
