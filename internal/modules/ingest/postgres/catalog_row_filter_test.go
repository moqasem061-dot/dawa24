package postgres

import (
	"reflect"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
)

func TestRowReloadUsesIDWithinImportRatherThanNameSearch(t *testing.T) {
	clause, args := rowsWhere(10, ingest.RowFilter{RowID: 37})
	if !strings.Contains(clause, "r.import_id = $1") || !strings.Contains(clause, "r.id = $2") || strings.Contains(clause, "ILIKE") {
		t.Fatalf("row reload is not scoped by import and row: %s", clause)
	}
	if !reflect.DeepEqual(args, []any{int64(10), int64(37)}) {
		t.Fatalf("unexpected row reload parameters: %#v", args)
	}
}
