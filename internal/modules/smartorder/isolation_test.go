package smartorder_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Architectural gates, asserted by reading the source.
//
// These are unusual tests, and they earn their place: each one guards a rule
// that is invisible at runtime until it has already cost something. A stray
// import compiles. A cart write succeeds. A float in a price path produces an
// invoice that is a piastre out and nobody notices for a month.

// FR-042. The review cart is not the shopping cart.
//
// An abandoned import must not leave items in a cart the buyer believes is
// empty, and a cart edit must not silently change a generated order. The two
// systems share no storage, and this is what keeps it that way when someone
// reaches for the familiar cart helper.
func TestSmartOrderNeverTouchesTheShoppingCart(t *testing.T) {
	forbidden := []string{"commerce.carts", "commerce.cart_items", "cart_items", "CartItem"}

	walkGoFiles(t, filepath.Join("..", "smartorder"), func(path, src string) {
		if strings.HasSuffix(path, "isolation_test.go") {
			return // this file names them in order to forbid them
		}
		for _, needle := range forbidden {
			if strings.Contains(stripComments(src), needle) {
				t.Errorf("%s references %q — the smart order review cart must not share storage "+
					"with the ordinary shopping cart (FR-042)", path, needle)
			}
		}
	})
}

// stripComments removes line comments so a rule stated in prose does not read as
// a violation of itself. Several files explain *why* they must not touch the
// cart, and naming it there is the explanation working, not the rule breaking.
func stripComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// AGENTS.md rule 5. The module must not import another module.
//
// Coverage, Corporate Operations, branch lookup and order placement all arrive
// as narrow function types the composition root fills in. An import here would
// be the first step back toward the tangle the module boundaries exist to
// prevent.
func TestSmartOrderImportsNoOtherModule(t *testing.T) {
	const prefix = `"github.com/muhiya/dawa24-store/internal/modules/`

	walkGoFiles(t, filepath.Join("..", "smartorder"), func(path, src string) {
		for _, line := range strings.Split(src, "\n") {
			idx := strings.Index(line, prefix)
			if idx < 0 {
				continue
			}
			rest := line[idx+len(prefix):]
			end := strings.Index(rest, `"`)
			if end < 0 {
				continue
			}
			imported := rest[:end]
			// Its own sub-packages are fine.
			if strings.HasPrefix(imported, "smartorder") {
				continue
			}
			t.Errorf("%s imports modules/%s — use an interface the composition root fills in "+
				"(AGENTS.md rule 5)", path, imported)
		}
	})
}

// AGENTS.md rule 1. Money never touches float64.
//
// The rule is about *money*, not about every number: quantities are counts,
// confidences and tolerances are ratios, and header-detection scores are
// similarity. What must never be a float is a price, a total, a net or an
// amount, because that is where a piastre goes missing and nobody notices for a
// month.
//
// So the assertion is targeted: no declaration binds a money-named identifier to
// a float64.
func TestMoneyIsNeverFloat(t *testing.T) {
	moneyFloat := regexp.MustCompile(`(?i)(price|total|net|amount|subtotal|budget)[a-z]*\s+\*?float64`)

	walkGoFiles(t, filepath.Join("..", "smartorder"), func(path, src string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for i, line := range strings.Split(src, "\n") {
			// DiscountPct is a display percentage, not an amount; it is named
			// so precisely to keep it distinguishable from a discount value.
			if strings.Contains(line, "DiscountPct") || strings.Contains(line, "TolerancePct") {
				continue
			}
			if moneyFloat.MatchString(line) {
				t.Errorf("%s:%d binds a money-named value to float64 — prices are money.Amount "+
					"(AGENTS.md rule 1): %s", path, i+1, strings.TrimSpace(line))
			}
		}
	})
}

// AGENTS.md rule 2. No provider or model name outside platform/gateway.
func TestNoProviderNamesInSmartOrder(t *testing.T) {
	providers := []string{"openai", "anthropic", "deepseek", "gemini", "groq", "openrouter", "qwen"}

	walkGoFiles(t, filepath.Join("..", "smartorder"), func(path, src string) {
		// This file names them in order to forbid them.
		if strings.HasSuffix(path, "isolation_test.go") {
			return
		}
		lower := strings.ToLower(src)
		for _, p := range providers {
			if strings.Contains(lower, p) {
				t.Errorf("%s names the provider %q — the module asks for a capability, never a "+
					"provider or model (AGENTS.md rule 2)", path, p)
			}
		}
	})
}

// walkGoFiles applies fn to every Go file under root.
func walkGoFiles(t *testing.T, root string, fn func(path, src string)) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fn(path, string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
