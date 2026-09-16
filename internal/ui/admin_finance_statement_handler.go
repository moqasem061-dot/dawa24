package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFinanceStatementPage renders a printable account statement for an organization.
func (h *UIHandler) AdminFinanceStatementPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil || orgID <= 0 {
		walletIDStr := strings.TrimSpace(r.URL.Query().Get("wallet_id"))
		walletID, wErr := strconv.ParseInt(walletIDStr, 10, 64)
		if wErr == nil && walletID > 0 && h.billSvc != nil {
			if wlt, _ := h.billSvc.GetWalletByID(ctx, walletID); wlt != nil {
				if wlt.OrganizationID != nil && *wlt.OrganizationID > 0 {
					orgID = *wlt.OrganizationID
				} else if h.orgSvc != nil && wlt.UserID > 0 {
					if allOrgs, _ := h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 1000, 0); len(allOrgs) > 0 {
						for _, o := range allOrgs {
							if o.OwnerID == wlt.UserID {
								orgID = o.ID
								break
							}
						}
					}
				}
			}
		}
	}

	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=transactions", "error", i18n.T(lang, "admin.finance.statement_select_org_required"))
		return
	}

	var targetOrg *org.Organization
	if h.orgSvc != nil {
		targetOrg, _ = h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID)
	}
	if targetOrg == nil {
		h.redirectWithNotice(w, r, "/admin/finance?tab=transactions", "error", i18n.T(lang, "admin.finance.organization_not_found"))
		return
	}

	var (
		transactions []*billing.AdminWalletTransactionView
		wallet       *billing.AdminWalletView
	)

	if h.billSvc != nil {
		wallets, _, _ := h.billSvc.AdminListDetailedWallets(ctx, billing.WalletFilter{
			OrganizationID: orgID,
			Limit:          1,
		})
		if len(wallets) > 0 {
			wallet = wallets[0]
		}

		transactions, _, _ = h.billSvc.AdminListDetailedTransactions(ctx, billing.TransactionFilter{
			OrganizationID: orgID,
			Limit:          1000,
			Offset:         0,
		})
	}

	h.renderPage(ctx, w, "render admin finance statement page", pages.AdminFinanceStatement(targetOrg, wallet, transactions, lang, dir))
}
