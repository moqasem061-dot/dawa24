package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Formatting one statement.

func creditStatementPath(v CreditStatementView) string {
	if v.Statement == nil || v.Statement.Purchase == nil {
		return "/vendor/offers-packages"
	}
	return fmt.Sprintf("/vendor/offers-packages/purchases/%d/statement", v.Statement.Purchase.ID)
}

// creditStatementSubtitle names the package and its window, so a vendor with
// three purchases of the same package can tell which statement they opened.
func creditStatementSubtitle(v CreditStatementView) string {
	p := v.Statement.Purchase
	if p == nil {
		return ""
	}
	name := fmt.Sprintf("#%d", p.PackageID)
	if p.Package != nil {
		if display := p.Package.Name.Get(i18n.ParseLang(v.Lang)); display != "" {
			name = display
		}
	}
	line := fmt.Sprintf("%s — %s ← %s",
		name, p.StartsAt.Format("2006-01-02"), p.ExpiresAt.Format("2006-01-02"))
	if v.OrgName != "" {
		line = v.OrgName + " · " + line
	}
	return line
}

// creditReasonLabel resolves a reason by key rather than by a switch, so the
// labels cannot drift from the database's CHECK constraint.
func creditReasonLabel(lang string, reason promo.CreditReason) string {
	return i18n.Translate(lang, "promo.credits.reason."+string(reason))
}

// creditDeltaLabel keeps the sign visible. A column of bare numbers cannot say
// which rows gave credits back.
func creditDeltaLabel(entry *promo.CreditEntry) string {
	if entry.Delta > 0 {
		return fmt.Sprintf("+%d", entry.Delta)
	}
	return fmt.Sprintf("%d", entry.Delta)
}

func creditDeltaClass(entry *promo.CreditEntry) string {
	if entry.IsRefund() {
		return "credit-delta-in"
	}
	return "credit-delta-out"
}

// creditEntryReference says what the movement was for. The note wins when there
// is one — it was written for a person to read — and the entity falls back to
// a type and id, which is still more than the counter said.
func creditEntryReference(lang string, entry *promo.CreditEntry) string {
	if entry.Note != "" {
		return entry.Note
	}
	if entry.EntityType == "" {
		return "—"
	}
	label := i18n.Translate(lang, "promo.credits.entity."+entry.EntityType)
	if label == "promo.credits.entity."+entry.EntityType {
		label = entry.EntityType
	}
	if entry.EntityID != nil {
		return fmt.Sprintf("%s #%d", label, *entry.EntityID)
	}
	return label
}
