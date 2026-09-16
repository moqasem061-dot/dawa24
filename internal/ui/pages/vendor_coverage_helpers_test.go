package pages

import (
	"encoding/json"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

func TestCoveragesToJSON(t *testing.T) {
	lat := 30.0444
	lon := 31.2357
	govID := int64(1)
	cityID := int64(10)
	from := "09:00"
	to := "17:00"

	coverages := []*workflow.CoverageView{
		{
			WeeklyCoverage: workflow.WeeklyCoverage{
				ID:             101,
				BranchID:       5,
				GovernorateID:  &govID,
				CityID:         &cityID,
				DayOfWeek:      0, // Sunday
				DistanceMeters: 15000,
				CoverageFrom:   &from,
				CoverageTo:     &to,
				Address:        "Cairo Downtown",
				Latitude:       &lat,
				Longitude:      &lon,
				IsActive:       true,
			},
			BranchName:        "Cairo Main Branch",
			GovernorateNameAr: "القاهرة",
			CityNameAr:        "وسط البلد",
		},
		{
			WeeklyCoverage: workflow.WeeklyCoverage{
				ID:             102,
				BranchID:       6,
				DayOfWeek:      6, // Saturday
				DistanceMeters: 800,
				IsActive:       false,
			},
			BranchName: "Giza Branch",
		},
	}

	jsonStr := coveragesToJSON(coverages)
	if jsonStr == "" || jsonStr == "[]" {
		t.Fatalf("expected non-empty JSON, got %q", jsonStr)
	}

	var items []coverageClientItem
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Item 0 assertions
	if items[0].ID != 101 {
		t.Errorf("expected ID 101, got %d", items[0].ID)
	}
	if items[0].BranchID != 5 {
		t.Errorf("expected BranchID 5, got %d", items[0].BranchID)
	}
	if items[0].BranchName != "Cairo Main Branch" {
		t.Errorf("expected BranchName 'Cairo Main Branch', got %q", items[0].BranchName)
	}
	if items[0].GovernorateName != "القاهرة" {
		t.Errorf("expected GovernorateName 'القاهرة', got %q", items[0].GovernorateName)
	}
	if items[0].CityName != "وسط البلد" {
		t.Errorf("expected CityName 'وسط البلد', got %q", items[0].CityName)
	}
	if items[0].DayNameAr != "الأحد" {
		t.Errorf("expected DayNameAr 'الأحد', got %q", items[0].DayNameAr)
	}
	if items[0].DistanceKM != "15.0 كم (15000 م)" {
		t.Errorf("expected DistanceKM '15.0 كم (15000 م)', got %q", items[0].DistanceKM)
	}
	if items[0].LatLngStr != "30.0444, 31.2357" {
		t.Errorf("expected LatLngStr '30.0444, 31.2357', got %q", items[0].LatLngStr)
	}
	if items[0].MapURL != "https://www.google.com/maps?q=30.044400,31.235700" {
		t.Errorf("expected MapURL, got %q", items[0].MapURL)
	}
	if !items[0].IsActive {
		t.Errorf("expected IsActive true")
	}

	// Item 1 assertions
	if items[1].ID != 102 {
		t.Errorf("expected ID 102, got %d", items[1].ID)
	}
	if items[1].GovernorateName != "مصر" {
		t.Errorf("expected default GovernorateName 'مصر', got %q", items[1].GovernorateName)
	}
	if items[1].DayNameAr != "السبت" {
		t.Errorf("expected DayNameAr 'السبت', got %q", items[1].DayNameAr)
	}
	if items[1].DistanceKM != "800 متر" {
		t.Errorf("expected DistanceKM '800 متر', got %q", items[1].DistanceKM)
	}
	if items[1].IsActive {
		t.Errorf("expected IsActive false")
	}
}
