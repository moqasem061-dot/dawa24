package postgres

import (
	"strings"
	"testing"
)

func TestNameExpr(t *testing.T) {
	expr := nameExpr("col")
	if !strings.Contains(expr, "col->>'ar'") {
		t.Errorf("expected Arabic fallback in %s", expr)
	}
	if !strings.Contains(expr, "col->>'en'") {
		t.Errorf("expected English fallback in %s", expr)
	}
	if !strings.Contains(expr, "col#>>'{}'") {
		t.Errorf("expected scalar JSON fallback in %s", expr)
	}
}
