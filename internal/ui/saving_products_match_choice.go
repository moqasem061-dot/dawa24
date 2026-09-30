package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// MatchChoice is an alias for catalog.SavingMatchChoice.
type MatchChoice = catalog.SavingMatchChoice

// MatchResult is an alias for catalog.SavingMatchResult.
type MatchResult = catalog.SavingMatchResult

// MatchStrategy is an alias for catalog.MatchStrategy.
type MatchStrategy = catalog.MatchStrategy

// SavingProductMatchEngine is an alias for catalog.SavingProductMatchEngine.
type SavingProductMatchEngine = catalog.SavingProductMatchEngine

var NewSavingProductMatchEngine = catalog.NewSavingProductMatchEngine

const (
	StrategyBarcodeOnly = catalog.StrategyBarcodeOnly
	StrategyIDOnly      = catalog.StrategyIDOnly
	StrategyNameOnly    = catalog.StrategyNameOnly
	StrategySKUBarcode  = catalog.StrategySKUBarcode
	StrategySmartAuto   = catalog.StrategySmartAuto
	StrategySKUOnly     = catalog.StrategySKUOnly
)

// DefaultMatchChoice returns the platform default matching configuration.
func DefaultMatchChoice() MatchChoice {
	return catalog.DefaultSavingMatchChoice()
}

// ParseMatchChoice reads the unified matching settings off a submitted form.
func ParseMatchChoice(r *http.Request) MatchChoice {
	c := DefaultMatchChoice()
	if raw := strings.TrimSpace(r.FormValue("min_match_score")); raw != "" {
		if pct, err := strconv.ParseFloat(raw, 64); err == nil && pct > 0 {
			if pct > 1 {
				pct /= 100
			}
			c.MinScore = pct
		}
	}
	c.ByBarcode = matchFlag(r, "match_by_barcode")
	c.ByCode = matchFlag(r, "match_by_code")
	c.CodeIsCatalogCode = matchFlag(r, "code_is_catalog_code")
	if r.FormValue("match_by_product_id") != "" {
		c.ByProductID = matchFlag(r, "match_by_product_id")
	}

	// Legacy: an open tab still posting one of the four exclusive strategies.
	switch MatchStrategy(strings.TrimSpace(r.FormValue("match_strategy"))) {
	case StrategyBarcodeOnly:
		c.ByBarcode = true
	case StrategySKUBarcode, StrategySKUOnly:
		c.ByBarcode, c.ByCode = true, true
	case StrategyIDOnly:
		c.ByProductID = true
	}
	return c.Normalize()
}

// ParseUseAI reads the AI switch. Absent means on.
func ParseUseAI(r *http.Request) bool {
	v := strings.TrimSpace(r.FormValue("use_ai"))
	if v == "" {
		return true
	}
	return v == "1" || v == "on" || v == "true"
}

// ParseUseAIFromWizard reads the AI switch off a form that actually drew it.
func ParseUseAIFromWizard(r *http.Request) bool {
	if strings.TrimSpace(r.FormValue("ai_choice")) == "" {
		return ParseUseAI(r)
	}
	return matchFlag(r, "use_ai")
}

// matchFlag reads a checkbox from either a urlencoded or a multipart body.
func matchFlag(r *http.Request, name string) bool {
	v := strings.TrimSpace(r.FormValue(name))
	return v == "1" || v == "on" || v == "true"
}
