package pages

import (
	"net/url"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

// Formatting the package-account screens.

// creditAccountName falls back to the id when a company has neither a trade
// name nor a legal name. A blank cell in a list you are meant to click through
// is worse than a number.
func creditAccountName(account *promo.CreditAccount) string {
	if account.OrganizationName != "" {
		return account.OrganizationName
	}
	return "#" + strconv.FormatInt(account.OrganizationID, 10)
}

func creditAccountLastPurchase(account *promo.CreditAccount) string {
	if account.LastPurchaseAt == nil {
		return "—"
	}
	return account.LastPurchaseAt.Format("2006-01-02")
}

// creditAccountsQuery keeps the search term across a page change.
func creditAccountsQuery(search string) url.Values {
	vals := url.Values{}
	if search != "" {
		vals.Set("q", search)
	}
	return vals
}

func purchaseStatusClass(purchase *promo.SponsorshipPurchase) string {
	switch purchase.Status {
	case "active":
		return "badge-emerald"
	case "expired":
		return "badge-slate"
	default:
		return "badge-amber"
	}
}
