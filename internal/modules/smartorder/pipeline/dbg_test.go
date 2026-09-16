package pipeline

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

func TestDebugGuard(t *testing.T) {
	idx := testIndex()
	l := &smartorder.Line{ID: 1, RawName: "ابليفاى 10مجم"}
	Normalize([]*smartorder.Line{l})
	row := BuildRow(l)
	t.Logf("row.Name=%q conc=%q form=%q", row.Name, row.Concentration, row.DosageForm)
	c := idx.IdentityConflict(row, 101)
	t.Logf("conflict kind=%q detail=%q", c.Kind, c.Detail)
}
