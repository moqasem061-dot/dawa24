package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// MonthSalesByVendor sums a vendor's sold order-line totals for the current
// month. The query runs tenant-scoped: order_lines.organization_id is the
// seller, so the caller's own tenant context already scopes it and the explicit
// predicate is the documented second guard.
func (r *Repository) MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error) {
	var total money.Amount
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COALESCE(SUM(total_price), 0)
			FROM commerce.order_lines
			WHERE organization_id = $1 AND created_at >= date_trunc('month', now());
		`
		return tx.QueryRow(txCtx, query, vendorOrgID).Scan(&total)
	})
	return total, err
}

// MonthSpendByCustomer sums what a buyer paid across all suppliers this month.
//
// This is deliberately cross-tenant: a pharmacy's purchases sit in order_lines
// owned by each supplier it bought from, so no single tenant context would see
// the whole month. The predicate is the guard here, not RLS.
func (r *Repository) MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error) {
	var total money.Amount
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COALESCE(SUM(ol.total_price), 0)
			FROM commerce.order_lines ol
			JOIN commerce.orders o ON o.id = ol.order_id
			WHERE o.customer_id = $1 AND ol.created_at >= date_trunc('month', now());
		`
		return tx.QueryRow(txCtx, query, customerID).Scan(&total)
	})
	return total, err
}

// GetVendorFinancialSummary calculates the complete, unified financial and profit metrics for a vendor.
// Orders that are delivered / completed have their discounted costs deducted from public selling prices to determine net profit.
// If a variant has no cost_price, its profit equals its selling price after public discount.
func (r *Repository) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string, filterOpt ...commerce.VendorFinancialFilter) (*commerce.VendorFinancialSummary, error) {
	summary := &commerce.VendorFinancialSummary{
		Period: period,
	}

	var filter commerce.VendorFinancialFilter
	if len(filterOpt) > 0 {
		filter = filterOpt[0]
	}
	summary.DateFrom = filter.DateFrom
	summary.DateTo = filter.DateTo
	summary.CustomerOrgID = filter.CustomerOrgID

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Determine date filter for delivered/confirmed shipments
		var dateFilter string
		args := []interface{}{vendorOrgID}
		argIdx := 2

		if filter.DateFrom != "" || filter.DateTo != "" {
			summary.Period = "custom"
			if filter.DateFrom != "" {
				dateFilter += fmt.Sprintf(" AND COALESCE(s.delivered_at, s.updated_at, s.created_at) >= $%d::date", argIdx)
				args = append(args, filter.DateFrom)
				argIdx++
			}
			if filter.DateTo != "" {
				dateFilter += fmt.Sprintf(" AND COALESCE(s.delivered_at, s.updated_at, s.created_at) < ($%d::date + interval '1 day')", argIdx)
				args = append(args, filter.DateTo)
				argIdx++
			}
		} else {
			switch period {
			case "last_month":
				dateFilter = " AND COALESCE(s.delivered_at, s.updated_at, s.created_at) >= date_trunc('month', now() - interval '1 month') AND COALESCE(s.delivered_at, s.updated_at, s.created_at) < date_trunc('month', now())"
			case "year":
				dateFilter = " AND COALESCE(s.delivered_at, s.updated_at, s.created_at) >= date_trunc('year', now())"
			case "all":
				dateFilter = ""
			default: // "month" or empty
				summary.Period = "month"
				dateFilter = " AND COALESCE(s.delivered_at, s.updated_at, s.created_at) >= date_trunc('month', now())"
			}
		}

		var orgFilter string
		if filter.CustomerOrgID > 0 {
			orgFilter = fmt.Sprintf(" AND (cust_org.id = $%d OR o.organization_id = $%d)", argIdx, argIdx)
			args = append(args, filter.CustomerOrgID)
			argIdx++
		}

		// 2. Query delivered and confirmed shipments for this vendor in the period
		queryShipments := `
			SELECT s.id, s.shipment_number, s.order_id, o.order_number,
			       COALESCE(cust_org.name->>'ar', cust_org.name->>'en', u.name->>'ar', u.name->>'en', 'صيدلية عميل') as customer_org_name,
			       COALESCE(s.delivered_at, s.updated_at, s.created_at) as delivered_at,
			       s.subtotal, s.shipping_fee, s.total_amount,
			       o.payment_status
			FROM commerce.order_shipments s
			JOIN commerce.orders o ON o.id = s.order_id
			LEFT JOIN org.organizations cust_org ON cust_org.id = o.organization_id
			LEFT JOIN identity.users u ON u.id = o.customer_id
			WHERE (s.organization_id = $1 
			       OR o.vendor_branch_id IN (SELECT id FROM org.branches WHERE organization_id = $1)
			       OR o.offer_id IN (SELECT id FROM promo.offers WHERE organization_id = $1)
			       OR EXISTS (SELECT 1 FROM commerce.order_lines ol WHERE ol.shipment_id = s.id AND ol.organization_id = $1))
			  AND (s.status IN ('delivered', 'completed', 'confirmed', 'shipped', 'out_for_delivery', 'in_transit') 
			       OR o.status IN ('delivered', 'completed', 'confirmed', 'shipped', 'out_for_delivery', 'in_transit'))
			  ` + dateFilter + orgFilter + `
			ORDER BY COALESCE(s.delivered_at, s.updated_at, s.created_at) DESC NULLS LAST, s.id DESC;
		`

		sRows, err := tx.Query(txCtx, queryShipments, args...)
		if err != nil {
			return err
		}
		defer sRows.Close()

		type shipRec struct {
			profit      *commerce.VendorShipmentProfit
			shipID      int64
			subtotal    money.Amount
			totalAmount money.Amount
		}
		var shipRecs []shipRec
		var shipIDs []int64

		for sRows.Next() {
			var s commerce.VendorShipmentProfit
			var subtotal, shippingFee, totalAmount money.Amount
			var payStatusStr string
			if err := sRows.Scan(
				&s.ShipmentID, &s.ShipmentNumber, &s.OrderID, &s.OrderNumber,
				&s.CustomerOrgName, &s.DeliveredAt,
				&subtotal, &shippingFee, &totalAmount,
				&payStatusStr,
			); err != nil {
				return err
			}
			s.PaymentStatus = payStatusStr
			shipRecs = append(shipRecs, shipRec{profit: &s, shipID: s.ShipmentID, subtotal: subtotal, totalAmount: totalAmount})
			shipIDs = append(shipIDs, s.ShipmentID)
		}
		sRows.Close()

		summary.DeliveredOrdersCount = len(shipRecs)

		// 3. Load order lines for all delivered shipments
		productMap := make(map[int64]*commerce.VendorProductProfit)
		shipmentLinesMap := make(map[int64][]*commerce.OrderLine)

		if len(shipIDs) > 0 {
			queryLines := `
				SELECT l.id, l.shipment_id, l.product_id, l.product_variant_id,
				       COALESCE(NULLIF(l.product_name->>'ar', ''), NULLIF(l.product_name->>'en', ''), 'صنف'),
				       COALESCE(l.sku, ''), l.unit_price, l.quantity, l.discount_amount, l.total_price,
				       COALESCE(l.cost_price, 0.00), COALESCE(l.cost_discount_percentage, 0.00),
				       COALESCE(l.list_price, 0.00), COALESCE(l.original_price, 0.00), COALESCE(l.original_discount, 0.00)
				FROM commerce.order_lines l
				WHERE l.shipment_id = ANY($1)
				ORDER BY l.id ASC;
			`
			lRows, err := tx.Query(txCtx, queryLines, shipIDs)
			if err != nil {
				return err
			}
			defer lRows.Close()

			for lRows.Next() {
				var l commerce.OrderLine
				var pName string
				var costPrice, listPrice, origPrice, origDiscount money.Amount
				if err := lRows.Scan(
					&l.ID, &l.ShipmentID, &l.ProductID, &l.ProductVariantID,
					&pName, &l.SKU, &l.UnitPrice, &l.Quantity, &l.DiscountAmount, &l.TotalPrice,
					&costPrice, &l.CostDiscountPercentage,
					&listPrice, &origPrice, &origDiscount,
				); err != nil {
					return err
				}
				if costPrice.IsPositive() {
					l.CostPrice = &costPrice
				}
				l.ListPrice = listPrice
				l.OriginalPrice = origPrice
				l.OriginalDiscount = origDiscount
				l.ProductName = i18n.New(pName, pName)
				shipmentLinesMap[l.ShipmentID] = append(shipmentLinesMap[l.ShipmentID], &l)
			}
			lRows.Close()
		}

		// Compute financials per shipment and aggregate into summary
		for _, rec := range shipRecs {
			sp := rec.profit
			lines := shipmentLinesMap[rec.shipID]
			sp.LineItemsCount = len(lines)

			var shipGross, shipDiscounts, shipNet, shipCOGS, shipProfit money.Amount

			if len(lines) == 0 {
				// Fallback when line-level rows aren't broken down in order_lines
				shipGross = rec.subtotal
				if shipGross.Minor() == 0 {
					shipGross = rec.totalAmount
				}
				shipNet = rec.totalAmount
				if shipNet.Minor() == 0 {
					shipNet = shipGross
				}
				if shipGross.Minor() > shipNet.Minor() {
					shipDiscounts = money.FromMinor(shipGross.Minor() - shipNet.Minor())
				}
				// When no cost records are available, cost is 0, profit is 0, and margin is 0.0%
				shipCOGS = money.Zero
				shipProfit = money.Zero
			} else {
				for _, line := range lines {
					pubPrice := line.EffectivePublicPrice()
					lineGross := money.FromMinor(pubPrice.Minor() * int64(line.Quantity))
					if lineGross.Minor() < line.TotalPrice.Minor() {
						lineGross = line.TotalPrice
					}
					lineDiscount := line.TotalSellingDiscount()
					lineCost := line.TotalCost()
					lineProfit := line.TotalNetProfit()

					shipGross, _ = shipGross.Add(lineGross)
					shipDiscounts, _ = shipDiscounts.Add(lineDiscount)
					shipNet, _ = shipNet.Add(line.TotalPrice)
					shipCOGS, _ = shipCOGS.Add(lineCost)
					shipProfit, _ = shipProfit.Add(lineProfit)

					// Aggregate product breakdown
					key := int64(0)
					if line.ProductVariantID != nil && *line.ProductVariantID > 0 {
						key = *line.ProductVariantID
					} else if line.ProductID != nil && *line.ProductID > 0 {
						key = *line.ProductID
					}
					if key > 0 {
						prod, exists := productMap[key]
						if !exists {
							prod = &commerce.VendorProductProfit{
								VariantID:              key,
								Name:                   line.ProductName.Get("ar"),
								SKU:                    line.SKU,
								PublicPrice:            pubPrice,
								SellingDiscount:        line.EffectiveSellingDiscountPercent(),
								SellingPrice:           line.EffectiveSellingPrice(),
								CostPrice:              line.CostPrice,
								CostDiscountPercentage: line.CostDiscountPercentage,
								DiscountedCost:         line.EffectivePurchaseCost(),
							}
							if line.ProductID != nil {
								prod.ProductID = *line.ProductID
							}
							productMap[key] = prod
						}
						prod.QuantitySold += line.Quantity
						prod.TotalRevenue, _ = prod.TotalRevenue.Add(line.TotalPrice)
						prod.TotalCost, _ = prod.TotalCost.Add(lineCost)
						prod.NetProfit, _ = prod.NetProfit.Add(lineProfit)
					}
				}
			}

			sp.GrossSales = shipGross
			sp.Discounts = shipDiscounts
			sp.NetSales = shipNet
			sp.COGS = shipCOGS
			sp.NetProfit = shipProfit
			if sp.COGS.IsPositive() && sp.NetSales.IsPositive() {
				sp.ProfitMargin = (float64(sp.NetProfit.Minor()) / float64(sp.NetSales.Minor())) * 100.0
			} else {
				sp.ProfitMargin = 0.0
			}
			sp.ProfitOnCost = 0.0

			summary.GrossSales, _ = summary.GrossSales.Add(shipGross)
			summary.TotalDiscounts, _ = summary.TotalDiscounts.Add(shipDiscounts)
			summary.NetSales, _ = summary.NetSales.Add(shipNet)
			summary.COGS, _ = summary.COGS.Add(shipCOGS)
			summary.NetProfit, _ = summary.NetProfit.Add(shipProfit)

			summary.Shipments = append(summary.Shipments, sp)
		}

		// Calculate profit margin on summary (Profit on Cost removed)
		if summary.COGS.IsPositive() && summary.NetSales.IsPositive() {
			summary.ProfitMargin = (float64(summary.NetProfit.Minor()) / float64(summary.NetSales.Minor())) * 100.0
		} else {
			summary.ProfitMargin = 0.0
		}
		summary.ProfitOnCost = 0.0

		// Convert product map to top products slice and calculate individual margins
		for _, prod := range productMap {
			if prod.TotalCost.IsPositive() && prod.TotalRevenue.IsPositive() {
				prod.ProfitMargin = (float64(prod.NetProfit.Minor()) / float64(prod.TotalRevenue.Minor())) * 100.0
			} else {
				prod.TotalCost = money.Zero
				prod.NetProfit = money.Zero
				prod.ProfitMargin = 0.0
			}
			prod.ProfitOnCost = 0.0
			summary.TopProducts = append(summary.TopProducts, prod)
		}

		// 4. Pending orders total and count
		pendingArgs := []interface{}{vendorOrgID}
		var pendingOrgFilter string
		if filter.CustomerOrgID > 0 {
			pendingOrgFilter = " AND (cust_org.id = $2 OR o.organization_id = $2)"
			pendingArgs = append(pendingArgs, filter.CustomerOrgID)
		}
		queryPending := `
			SELECT COUNT(*), COALESCE(SUM(s.total_amount), 0)
			FROM commerce.order_shipments s
			JOIN commerce.orders o ON o.id = s.order_id
			LEFT JOIN org.organizations cust_org ON cust_org.id = o.organization_id
			WHERE (s.organization_id = $1 
			       OR o.vendor_branch_id IN (SELECT id FROM org.branches WHERE organization_id = $1)
			       OR o.offer_id IN (SELECT id FROM promo.offers WHERE organization_id = $1)
			       OR EXISTS (SELECT 1 FROM commerce.order_lines ol WHERE ol.shipment_id = s.id AND ol.organization_id = $1))
			  AND (s.status IN ('pending', 'processing', 'on_hold') OR o.status IN ('pending', 'processing', 'on_hold'))` + pendingOrgFilter + `;
		`
		_ = tx.QueryRow(txCtx, queryPending, pendingArgs...).Scan(&summary.PendingOrdersCount, &summary.PendingOrdersTotal)

		// 5. Wallet balance
		queryWallet := `
			SELECT COALESCE(
				(SELECT wt.balance_after
				 FROM billing.wallet_transactions wt
				 JOIN billing.wallets w ON w.id = wt.wallet_id
				 WHERE (w.organization_id = $1
				        OR w.user_id = (SELECT owner_id FROM org.organizations WHERE id = $1)
				        OR w.user_id IN (SELECT user_id FROM org.members WHERE organization_id = $1 AND role_key IN ('owner', 'org_owner')))
				 ORDER BY wt.id DESC
				 LIMIT 1),
			0);
		`
		if err := tx.QueryRow(txCtx, queryWallet, vendorOrgID).Scan(&summary.WalletBalance); err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("dashboard: wallet balance: %w", err)
		}

		return nil
	})

	return summary, err
}
