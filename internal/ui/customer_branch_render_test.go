package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestCustomerBranchesRender(t *testing.T) {
	lat := 30.0444
	lng := 31.2357
	cityID := int64(1)
	branch := &org.Branch{
		ID:                 12,
		OrganizationID:     5,
		Name:               i18n.New("صيدلية النصر \"المعادي\"", "Al Nasr \"Maadi\""),
		Code:               "PH-01",
		Address:            "شارع 9، المعادي، القاهرة",
		Phone:              "01012345678",
		OperatingHours:     "24 ساعة",
		GoogleMapsURL:      "https://maps.google.com/?q=30.0444,31.2357",
		HasColdStorage:     true,
		IsMain:             true,
		CityID:             &cityID,
		Latitude:           &lat,
		Longitude:          &lng,
		InstitutionalWorks: []string{"hospitals", "centers"},
	}

	data := pages.CustomerBranchesData{
		Branches:       []*org.Branch{branch},
		StaffPerBranch: map[int64]int{12: 3},
		TotalStaff:     3,
		Cities: []*platformadmin.City{
			{ID: 1, Name: i18n.New("القاهرة", "Cairo"), Latitude: 30.0444, Longitude: 31.2357},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()
	err := pages.CustomerBranches(data, "ar", "rtl", []string{"*master*"}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	html := buf.String()
	for _, line := range strings.Split(html, "\n") {
		if strings.Contains(line, "data-edit-branch-btn") || strings.Contains(line, "setEditMode") {
			t.Logf("RENDERED LINE:\n%s\n", line)
		}
	}
}
