package ui

import (
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorDashboardPage renders the supplier dashboard.
func (h *UIHandler) VendorDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.VendorDashboardData{
		Subscription: h.loadOrgSubscriptionView(ctx, actor, lang),
	}

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branches {
				if b != nil && (b.Status == "active" || b.Status == "") {
					data.ActiveWarehousesCount++
				}
			}
		}
	}

	if h.attSvc != nil && actor.OrganizationID > 0 {
		if reqs, err := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID); err == nil {
			for _, reqItem := range reqs {
				if reqItem != nil && reqItem.Status == attachments.DocReqPending {
					data.PendingDocRequests = append(data.PendingDocRequests, reqItem)
				}
			}
		}
	}

	if h.catSvc != nil {
		if n, err := h.catSvc.CountProductsByOrg(ctx, actor.OrganizationID, string(catalog.StatusActive)); err != nil {
			h.log.ErrorContext(ctx, "vendor dashboard: count products", "error", err)
		} else {
			data.ActiveProducts = n
		}
	}

	if h.commSvc != nil {
		if shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 150, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list shipments", "error", err)
		} else {
			data.TotalShipmentsCount = len(shipments)
			for _, sh := range shipments {
				if sh == nil {
					continue
				}
				if sh.Status == commerce.StatusDelivered || sh.Status == commerce.StatusCompleted {
					data.DeliveredShipments++
				} else if sh.Status == commerce.StatusPending || sh.Status == commerce.StatusConfirmed || sh.Status == commerce.StatusProcessing {
					data.PendingOrdersTotal, _ = data.PendingOrdersTotal.Add(sh.TotalAmount)
					if len(data.Shipments) < 12 {
						data.Shipments = append(data.Shipments, sh)
					}
				}
			}
			data.PendingShipments = len(data.Shipments)
		}
		if finSummary, err := h.commSvc.GetVendorFinancialSummary(ctx, actor.OrganizationID, "month"); err == nil && finSummary != nil {
			data.MonthSales = finSummary.NetSales
			data.MonthNetProfit = finSummary.NetProfit
			data.MonthProfitMargin = finSummary.ProfitMargin
			data.MonthCOGS = finSummary.COGS
			if finSummary.DeliveredOrdersCount > 0 {
				data.DeliveredShipments = finSummary.DeliveredOrdersCount
			}
			if finSummary.PendingOrdersCount > 0 {
				data.PendingOrdersTotal = finSummary.PendingOrdersTotal
			}
			if finSummary.WalletBalance.IsPositive() {
				data.WalletBalance = finSummary.WalletBalance
				data.HasWallet = true
			}
		} else {
			if total, err := h.commSvc.MonthSalesByVendor(ctx, actor.OrganizationID); err == nil {
				data.MonthSales = total
			}
		}
		if quotes, err := h.commSvc.ListQuoteRequests(ctx, actor.OrganizationID, true, 100, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list quote requests", "error", err)
		} else {
			for _, q := range quotes {
				if q.Status == commerce.QuotePending {
					data.UnreadQuotes++
				}
			}
		}
	}

	if h.invSvc != nil && actor.OrganizationID > 0 {
		allOrgStocks, err := h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list stocks by org", "error", err)
		} else {
			var criticalStocks []*inventory.Stock
			for _, s := range allOrgStocks {
				// Critical items (أصناف حرجة) are strictly products registered in a warehouse whose quantity reached 0
				if s != nil && s.Quantity <= 0 {
					criticalStocks = append(criticalStocks, s)
				}
			}
			data.LowStockCount = len(criticalStocks)
			if len(criticalStocks) > 5 {
				data.LowStock = criticalStocks[:5]
			} else {
				data.LowStock = criticalStocks
			}
			if h.catSvc != nil && len(data.LowStock) > 0 {
				data.LowStockProductNames = make(map[int64]string, len(data.LowStock))
				for _, s := range data.LowStock {
					if _, exists := data.LowStockProductNames[s.ProductID]; !exists {
						if prod, _, err := h.catSvc.GetProduct(ctx, s.ProductID); err == nil && prod != nil {
							data.LowStockProductNames[s.ProductID] = prod.Name.Get(i18n.Lang(lang))
						}
					}
				}
			}
		}
	}
	if h.catSvc != nil && actor.OrganizationID > 0 {
		if vStats, err := h.catSvc.VendorVariantStats(ctx, actor.OrganizationID); err == nil {
			if vStats.Active > 0 && data.ActiveProducts == 0 {
				data.ActiveProducts = vStats.Active
			}
		}
	}

	if h.promoSvc != nil {
		// The strip manages this supplier's own promotions, with their views
		// and clicks; other suppliers' offers and counters are not theirs.
		if offers, err := h.promoSvc.ListRunningOffersByOrg(ctx, actor.OrganizationID, 10); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list active offers", "error", err)
		} else {
			data.Offers = offers
		}
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "vendor dashboard: get wallet optional", "error", err)
		} else if wallet != nil {
			data.WalletBalance = wallet.Available()
			data.HasWallet = true
		}
	}

	h.renderPage(ctx, w, "render vendor dashboard", pages.VendorDashboard(lang, dir, data, actor))
}

// PharmacyDashboardPage renders the pharmacy buyer dashboard.
func (h *UIHandler) PharmacyDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.PharmacyDashboardData{
		Subscription: h.loadOrgSubscriptionView(ctx, actor, lang),
	}

	// 1. Pharmacy Organization & Branches
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		sysCtx := database.AsSystem(ctx)
		if o, err := h.orgSvc.GetOrganization(sysCtx, actor.OrganizationID); err == nil && o != nil {
			data.CustomerOrgName = o.LegalName
			if data.CustomerOrgName == "" && !o.TradeName.IsEmpty() {
				data.CustomerOrgName = o.TradeName.Get(i18n.Lang(lang))
			}
		}
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			data.Branches = branches
			data.TotalBranches = len(branches)
			for _, b := range branches {
				if b != nil && b.Status == "active" {
					data.ActiveBranches++
				}
			}
		}
		activeTargetID := int64(0)
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
			activeTargetID = *buying.Active
		} else if actor.BranchID != nil && *actor.BranchID > 0 {
			activeTargetID = *actor.BranchID
		}
		if activeTargetID > 0 {
			data.ActiveBranchID = activeTargetID
			for _, b := range data.Branches {
				if b.ID == activeTargetID {
					data.ActiveBranchName = b.Name.Get(i18n.Lang(lang))
					if data.ActiveBranchName == "" {
						data.ActiveBranchName = b.Code
					}
					break
				}
			}
		}
		if data.ActiveBranchName == "" && len(data.Branches) > 0 {
			data.ActiveBranchID = data.Branches[0].ID
			data.ActiveBranchName = data.Branches[0].Name.Get(i18n.Lang(lang))
		}
	}

	// 2. Urgent Administrative Document Requests
	if h.attSvc != nil && actor.OrganizationID > 0 {
		if reqs, err := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID); err == nil {
			for _, reqItem := range reqs {
				if reqItem != nil && reqItem.Status == attachments.DocReqPending {
					data.PendingDocRequests = append(data.PendingDocRequests, reqItem)
				}
			}
		}
	}

	// 3. Orders, Spend & Item totals
	if h.commSvc != nil {
		if orders, err := h.commSvc.ListCustomerOrders(ctx, actor.UserID, 100, 0); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list orders", "error", err)
		} else {
			data.TotalOrders = len(orders)
			branchMap := make(map[int64]*org.Branch)
			for _, b := range data.Branches {
				branchMap[b.ID] = b
			}

			for i, o := range orders {
				if o.BranchID != nil && *o.BranchID > 0 {
					if b, ok := branchMap[*o.BranchID]; ok && b != nil {
						if o.CustomerBranchName.IsEmpty() {
							o.CustomerBranchName = b.Name
						}
					}
				}
				if o.Status == commerce.StatusDelivered || o.Status == commerce.StatusCompleted {
					data.CompletedOrders++
					data.TotalSpend, _ = data.TotalSpend.Add(o.TotalAmount)
				} else if o.Status == commerce.StatusCancelled || o.Status == commerce.StatusFailed || o.Status == commerce.StatusReturned || o.Status == commerce.StatusRefunded {
					data.CancelledOrders++
				} else {
					data.ActiveOrders++
					data.TotalSpend, _ = data.TotalSpend.Add(o.TotalAmount)
				}

				if i < 8 {
					data.Orders = append(data.Orders, o)
				}
			}

			// The recent orders show a line count, which one grouped query
			// answers; loading each order in full cost four queries apiece.
			recentIDs := make([]int64, 0, len(data.Orders))
			for _, o := range data.Orders {
				recentIDs = append(recentIDs, o.ID)
			}
			if counts, err := h.commSvc.CountOrderLines(ctx, recentIDs); err == nil {
				for _, o := range data.Orders {
					o.LineCount = counts[o.ID]
					data.TotalOrderedItems += o.LineCount
				}
			}
			if data.TotalOrderedItems == 0 && data.TotalOrders > 0 {
				data.TotalOrderedItems = data.TotalOrders
			}
		}
		if total, err := h.commSvc.MonthSpendByCustomer(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: month spend", "error", err)
		} else {
			data.MonthSpend = total
		}
	}

	// 4. Wallet, Balance, Pending Deposits & Transactions
	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "pharmacy dashboard: get wallet optional", "error", err)
		} else if wallet != nil {
			data.WalletBalance = wallet.Available()
			data.HasWallet = true

			if txs, err := h.billSvc.ListWalletTransactions(ctx, wallet.ID, 5, 0); err == nil {
				data.RecentTransactions = txs
			}
		}

		if deposits, err := h.billSvc.ListUserDeposits(ctx, actor.UserID, 50, 0); err == nil {
			for _, dep := range deposits {
				if dep != nil && dep.Status == billing.DepositPending {
					data.PendingDepositsCount++
					data.PendingDepositsTotal, _ = data.PendingDepositsTotal.Add(dep.Amount)
				}
			}
		}
	}

	// 5. Smart Orders Lifecycle & Recent Runs
	if h.smartOrderSvc != nil && actor.OrganizationID > 0 {
		if runs, err := h.smartOrderSvc.History(ctx, actor.OrganizationID, 50, 0); err == nil {
			data.SmartOrdersTotal = len(runs)
			data.SmartOrdersCount = len(runs)
			for _, r := range runs {
				if r == nil {
					continue
				}
				switch r.Status {
				case smartorder.StatusProcessing, smartorder.StatusQueued:
					data.SmartOrdersProcessing++
				case smartorder.StatusPlaced, smartorder.StatusCompleted, smartorder.StatusFinalizing:
					data.SmartOrdersCompleted++
				case smartorder.StatusDraft, smartorder.StatusMapping, smartorder.StatusStale:
					data.SmartOrdersNeedsReview++
				}
				if len(data.RecentSmartOrders) < 4 {
					data.RecentSmartOrders = append(data.RecentSmartOrders, r)
				}
			}
		}
	}

	// 6. Favorites & Active Offers
	if h.idSvc != nil {
		if favs, err := h.idSvc.ListFavorites(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list favorites", "error", err)
		} else {
			data.Favorites = len(favs)
		}
	}

	if h.promoSvc != nil {
		if buyer := h.buyerOfferQuery(ctx, actor, 0); buyer.Notice == "" {
			q := buyer.Query
			q.Limit = 6
			if offers, total, err := h.promoSvc.ListBuyerOffers(ctx, q); err != nil {
				h.log.WarnContext(ctx, "pharmacy dashboard: buyer offers", "error", err)
			} else {
				data.Offers, data.ActiveOffers = offers, total
			}
		}
	}

	h.renderPage(ctx, w, "render pharmacy dashboard", pages.PharmacyDashboard(lang, dir, data, actor.Permissions))
}

// aiLedgerWindow is how far back the usage screens look.
//
// A month rather than everything: it is the period a budget window covers and
// the period a pharmacy reasons about when deciding whether their plan fits.
// The rows older than this are still in the ledger and still queryable.
const aiLedgerWindow = 30 * 24 * time.Hour
