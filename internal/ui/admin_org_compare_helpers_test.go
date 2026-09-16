package ui_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestAdminCompare_URLHelpers(t *testing.T) {
	orgID := int64(192)

	t.Run("CompareResultsPageData", func(t *testing.T) {
		data := pages.CompareResultsPageData{
			IsAdmin:         true,
			TargetOrgID:     orgID,
			TargetOrgName:   "مستودع الأمل",
			SuppliersParam:  "10,11",
		}
		if url := data.FileCenterURL(); url != "/admin/organizations/import/192/compare" {
			t.Fatalf("expected /admin/organizations/import/192/compare, got %s", url)
		}
		if sub := data.SubtabURL("/compare/head-to-head"); !strings.Contains(sub, "org_id=192") {
			t.Fatalf("expected SubtabURL to include org_id=192, got %s", sub)
		}
		if flt := data.FilterURL("all"); !strings.Contains(flt, "org_id=192") {
			t.Fatalf("expected FilterURL to include org_id=192, got %s", flt)
		}
	})

	t.Run("HeadToHeadPageData", func(t *testing.T) {
		data := pages.HeadToHeadPageData{
			IsAdmin:       true,
			TargetOrgID:   orgID,
			TargetOrgName: "مستودع الأمل",
		}
		if url := data.FileCenterURL(); url != "/admin/organizations/import/192/compare" {
			t.Fatalf("expected /admin/organizations/import/192/compare, got %s", url)
		}
		if sub := data.SubtabURL("/compare/market-benchmark"); !strings.Contains(sub, "org_id=192") {
			t.Fatalf("expected SubtabURL to include org_id=192, got %s", sub)
		}
	})

	t.Run("MarketBenchmarkPageData", func(t *testing.T) {
		data := pages.MarketBenchmarkPageData{
			IsAdmin:       true,
			TargetOrgID:   orgID,
			TargetOrgName: "مستودع الأمل",
		}
		if url := data.FileCenterURL(); url != "/admin/organizations/import/192/compare" {
			t.Fatalf("expected /admin/organizations/import/192/compare, got %s", url)
		}
		if sub := data.SubtabURL("/compare/market-intelligence"); !strings.Contains(sub, "org_id=192") {
			t.Fatalf("expected SubtabURL to include org_id=192, got %s", sub)
		}
	})

	t.Run("MarketIntelligencePageData", func(t *testing.T) {
		data := pages.MarketIntelligencePageData{
			IsAdmin:       true,
			TargetOrgID:   orgID,
			TargetOrgName: "مستودع الأمل",
			Query:         "panadol",
			SingleQuery:   "aspirin",
		}
		if url := data.FileCenterURL(); url != "/admin/organizations/import/192/compare" {
			t.Fatalf("expected /admin/organizations/import/192/compare, got %s", url)
		}
		if sub := data.SubtabURL("/compare/results"); !strings.Contains(sub, "org_id=192") {
			t.Fatalf("expected SubtabURL to include org_id=192, got %s", sub)
		}
		if clr := data.ClearQueryURL(); !strings.Contains(clr, "org_id=192") || !strings.Contains(clr, "q_single=aspirin") {
			t.Fatalf("expected ClearQueryURL to preserve org_id and q_single, got %s", clr)
		}
		if clrSingle := data.ClearSingleQueryURL(); !strings.Contains(clrSingle, "org_id=192") || !strings.Contains(clrSingle, "q=panadol") {
			t.Fatalf("expected ClearSingleQueryURL to preserve org_id and q, got %s", clrSingle)
		}
	})
}
