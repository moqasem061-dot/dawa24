package postgres

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestWarehouseStatusFilter(t *testing.T) {
	for _, tc := range []struct {
		status, clause string
		args           int
	}{
		{"", "", 0}, {"active", "f.status != 'archived'", 0}, {"archived", "f.status = $1", 1}, {"ready", "f.status = $1", 1},
	} {
		t.Run(tc.status, func(t *testing.T) {
			filter := compare.AdminTempWarehouseFilter{}
			if tc.status != "" {
				status := compare.CompareFileStatus(tc.status)
				filter.Status = &status
			}
			clauses, args := buildAdminTempWarehouseWhere(filter)
			if !strings.Contains(strings.Join(clauses, " AND "), tc.clause) || len(args) != tc.args {
				t.Fatalf("clauses=%v args=%v", clauses, args)
			}
		})
	}
}

func TestActiveStatusDoesNotShiftUploaderParameters(t *testing.T) {
	status := compare.CompareFileStatus("active")
	uploader := int64(7)
	clauses, args := buildAdminTempWarehouseWhere(compare.AdminTempWarehouseFilter{Status: &status, Search: "supplier", UploaderID: &uploader, Source: "moderator"})
	query := strings.Join(clauses, " AND ")
	if !strings.Contains(query, "f.user_id = $2") || !strings.Contains(query, "f.organization_id IS NULL") || len(args) != 2 {
		t.Fatalf("clauses=%v args=%v", clauses, args)
	}
}
