package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// buildPrintableInvoiceData assembles the full printable invoice data model,
// including resolution of item codes, pharmaceutical batch numbers, and expiry dates.
func (h *UIHandler) buildPrintableInvoiceData(ctx context.Context, invoice *billing.Invoice, order *commerce.Order, lang any) (*billing.PrintableInvoiceData, error) {
	if invoice == nil && order == nil {
		return nil, fmt.Errorf("both invoice and order are nil")
	}

	if invoice == nil && order != nil && h.billSvc != nil {
		if inv, err := h.billSvc.GetInvoiceByOrderID(ctx, order.ID); err == nil && inv != nil {
			invoice = inv
		} else {
			invoice = h.synthesizeInvoiceFromOrder(ctx, order)
		}
	}

	if order == nil && invoice != nil && invoice.OrderID != nil && h.commSvc != nil {
		if ord, _ := h.commSvc.GetOrder(ctx, *invoice.OrderID); ord != nil {
			order = ord
		}
	}

	vendorInfo := h.buildPrintableVendorInfo(ctx, invoice, lang)
	custInfo := h.buildPrintableCustomerInfo(ctx, invoice, order, lang)
	printableLines := h.buildPrintableLines(ctx, invoice, order)

	invNumber := "INV-2026-00001"
	var issueDate, dueDate time.Time
	var subtotal, totalDiscount, totalTax, totalAmount money.Amount
	invStatus := billing.InvoicePaid
	paymentMethod := "credit"
	paymentStatus := "paid"
	notes := ""
	var orderNum string

	if invoice != nil {
		invNumber = invoice.InvoiceNumber
		issueDate = invoice.IssueDate
		dueDate = invoice.DueDate
		subtotal = invoice.Subtotal
		totalDiscount = invoice.DiscountAmount
		totalTax = invoice.TaxAmount
		totalAmount = invoice.TotalAmount
		invStatus = invoice.Status
		paymentMethod = invoice.PaymentMethod
		notes = invoice.Notes
	}

	if order != nil {
		orderNum = order.OrderNumber
		if issueDate.IsZero() {
			issueDate = order.CreatedAt
		}
		if order.Subtotal.IsPositive() {
			subtotal = order.Subtotal
		} else if subtotal.IsZero() {
			subtotal = order.Subtotal
		}
		if order.TotalDiscount.IsPositive() {
			totalDiscount = order.TotalDiscount
		} else if totalDiscount.IsZero() {
			totalDiscount = order.TotalDiscount
		}
		if order.TotalAmount.IsPositive() {
			totalAmount = order.TotalAmount
		} else if totalAmount.IsZero() {
			totalAmount = order.TotalAmount
		}
		paymentMethod = string(order.PaymentMethod)
		paymentStatus = string(order.PaymentStatus)
		if notes == "" {
			notes = order.Notes
		}
	}

	var calcGross, calcNet, calcDiscount money.Amount
	for _, pl := range printableLines {
		lg, _ := pl.UnitPrice.MulInt(int64(pl.Quantity))
		calcGross, _ = calcGross.Add(lg)
		calcNet, _ = calcNet.Add(pl.TotalPrice)
		ld, _ := lg.Sub(pl.TotalPrice)
		if ld.IsPositive() {
			calcDiscount, _ = calcDiscount.Add(ld)
		}
	}
	if calcGross.IsPositive() {
		subtotal = calcGross
		totalDiscount = calcDiscount
		totalAmount, _ = calcNet.Add(totalTax)
	} else {
		if subtotal.IsZero() || (subtotal.Minor() <= totalAmount.Minor() && calcDiscount.IsPositive()) {
			subtotal = calcGross
		}
		if totalDiscount.IsZero() && calcDiscount.IsPositive() {
			totalDiscount = calcDiscount
		}
		if totalAmount.IsZero() {
			tot, _ := subtotal.Sub(totalDiscount)
			tot, _ = tot.Add(totalTax)
			totalAmount = tot
		}
	}

	var deliveryCode, trackingNumber string
	if order != nil {
		for _, s := range order.Shipments {
			if s != nil {
				if deliveryCode == "" && s.DeliveryCode != "" {
					deliveryCode = s.DeliveryCode
				}
				if trackingNumber == "" && s.TrackingNumber != "" {
					trackingNumber = s.TrackingNumber
				}
			}
		}
	}

	if issueDate.IsZero() {
		issueDate = time.Now().UTC()
	}
	if dueDate.IsZero() {
		dueDate = issueDate.AddDate(0, 0, 30)
	}

	qrData := url.QueryEscape(fmt.Sprintf("Dawa24|Seller:%s|TaxID:%s|Invoice:%s|Date:%s|Total:%s|VAT:0.00",
		vendorInfo.DisplayName, vendorInfo.TaxNumber, invNumber, issueDate.Format("2006-01-02T15:04:05Z"), totalAmount.String()))

	return &billing.PrintableInvoiceData{
		InvoiceID:      0,
		InvoiceNumber:  invNumber,
		OrderNumber:    orderNum,
		IssueDate:      issueDate,
		DueDate:        dueDate,
		Vendor:         vendorInfo,
		Customer:       custInfo,
		Lines:          printableLines,
		Subtotal:       subtotal,
		TotalDiscount:  totalDiscount,
		TaxableAmount:  totalAmount,
		VATAmtExempt:   money.Zero,
		VATAmtStandard: money.Zero,
		TotalTax:       totalTax,
		TotalAmount:    totalAmount,
		Status:         invStatus,
		PaymentMethod:  paymentMethod,
		PaymentStatus:  paymentStatus,
		DeliveryCode:   deliveryCode,
		TrackingNumber: trackingNumber,
		Notes:          notes,
		QRCodeData:     qrData,
	}, nil
}

func (h *UIHandler) buildPrintableLines(ctx context.Context, invoice *billing.Invoice, order *commerce.Order) []*billing.PrintableInvoiceLine {
	var varIDs []int64
	var prodIDs []int64
	if order != nil {
		for _, ol := range order.Lines {
			if ol.ProductVariantID != nil && *ol.ProductVariantID > 0 {
				varIDs = append(varIDs, *ol.ProductVariantID)
			}
			if ol.ProductID != nil && *ol.ProductID > 0 {
				prodIDs = append(prodIDs, *ol.ProductID)
			}
		}
	}
	if invoice != nil {
		for _, il := range invoice.Lines {
			if il.ProductID != nil && *il.ProductID > 0 {
				prodIDs = append(prodIDs, *il.ProductID)
			}
		}
	}

	var variantsMap map[int64]*catalog.ProductVariant
	var prodVariantsMap map[int64][]*catalog.ProductVariant
	if h.catSvc != nil {
		if len(varIDs) > 0 {
			variantsMap, _ = h.catSvc.GetVariantsByIDs(ctx, varIDs)
		}
		if len(prodIDs) > 0 {
			prodVariantsMap, _ = h.catSvc.ListVariantsByProducts(ctx, prodIDs)
		}
	}

	var printableLines []*billing.PrintableInvoiceLine
	if invoice != nil && len(invoice.Lines) > 0 {
		for idx, l := range invoice.Lines {
			var matchedVariantID *int64
			lineSKU := ""
			var lListPrice, lOrigDisc, lDiscAmount money.Amount
			if order != nil && idx < len(order.Lines) {
				ol := order.Lines[idx]
				matchedVariantID = ol.ProductVariantID
				lineSKU = ol.SKU
				lListPrice = ol.ListPrice
				lOrigDisc = ol.OriginalDiscount
				lDiscAmount = ol.DiscountAmount
			}

			var matchedVariant *catalog.ProductVariant
			if matchedVariantID != nil && variantsMap != nil {
				matchedVariant = variantsMap[*matchedVariantID]
			}

			unitPrice, discPct, netPrice := resolvePrintableLinePricing(
				l.UnitPrice, l.TotalPrice, l.Quantity, lListPrice, lOrigDisc, lDiscAmount, matchedVariant,
			)

			sku, batch, expiry := resolveVariantLineInfo(l.ProductID, matchedVariantID, lineSKU, variantsMap, prodVariantsMap)
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.Description,
				SKU:             sku,
				BatchNumber:     batch,
				ExpiryDate:      expiry,
				Quantity:        l.Quantity,
				UnitPrice:       unitPrice,
				DiscountPercent: discPct,
				NetUnitPrice:    netPrice,
				TotalPrice:      l.TotalPrice,
				IsExempt:        true,
			})
		}
	} else if order != nil && len(order.Lines) > 0 {
		for idx, l := range order.Lines {
			var matchedVariant *catalog.ProductVariant
			if l.ProductVariantID != nil && variantsMap != nil {
				matchedVariant = variantsMap[*l.ProductVariantID]
			}

			unitPrice, discPct, netPrice := resolvePrintableLinePricing(
				l.UnitPrice, l.TotalPrice, l.Quantity, l.ListPrice, l.OriginalDiscount, l.DiscountAmount, matchedVariant,
			)

			sku, batch, expiry := resolveVariantLineInfo(l.ProductID, l.ProductVariantID, l.SKU, variantsMap, prodVariantsMap)
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.ProductName.Get("ar"),
				SKU:             sku,
				BatchNumber:     batch,
				ExpiryDate:      expiry,
				Quantity:        l.Quantity,
				UnitPrice:       unitPrice,
				DiscountPercent: discPct,
				NetUnitPrice:    netPrice,
				TotalPrice:      l.TotalPrice,
				IsExempt:        true,
			})
		}
	}
	return printableLines
}

func resolveVariantLineInfo(
	productID *int64,
	variantID *int64,
	existingSKU string,
	variantsMap map[int64]*catalog.ProductVariant,
	prodVariantsMap map[int64][]*catalog.ProductVariant,
) (sku, batch, expiry string) {
	sku = existingSKU
	if variantID != nil && variantsMap != nil {
		if v, ok := variantsMap[*variantID]; ok && v != nil {
			if sku == "" {
				sku = v.SKU
			}
			batch = v.BatchNumber
			if v.ExpiryDate != nil && !v.ExpiryDate.IsZero() {
				expiry = v.ExpiryDate.Format("2006-01-02")
			}
		}
	}
	if (batch == "" || expiry == "" || sku == "") && productID != nil && prodVariantsMap != nil {
		if list, ok := prodVariantsMap[*productID]; ok && len(list) > 0 {
			for _, v := range list {
				if v == nil {
					continue
				}
				if sku == "" && v.SKU != "" {
					sku = v.SKU
				}
				if batch == "" && v.BatchNumber != "" {
					batch = v.BatchNumber
				}
				if expiry == "" && v.ExpiryDate != nil && !v.ExpiryDate.IsZero() {
					expiry = v.ExpiryDate.Format("2006-01-02")
				}
				if sku != "" && batch != "" && expiry != "" {
					break
				}
			}
		}
	}
	return sku, batch, expiry
}

func resolvePrintableLinePricing(
	lUnitPrice money.Amount,
	lTotalPrice money.Amount,
	qty int,
	lListPrice money.Amount,
	lOrigDisc money.Amount,
	lDiscAmount money.Amount,
	matchedVariant *catalog.ProductVariant,
) (unitPrice money.Amount, discPct float64, netPrice money.Amount) {
	discPct = 0.0
	if lOrigDisc.IsPositive() {
		discPct = float64(lOrigDisc.Minor()) / 100.0
	} else if matchedVariant != nil && matchedVariant.Discount.IsPositive() {
		discPct = matchedVariant.DiscountPercentageFloat()
	} else if lDiscAmount.IsPositive() && lUnitPrice.IsPositive() && qty > 0 {
		grossMinor := lUnitPrice.Minor() * int64(qty)
		if grossMinor > 0 {
			discPct = float64(lDiscAmount.Minor()) / float64(grossMinor) * 100.0
		}
	}

	unitPrice = lUnitPrice
	if lListPrice.IsPositive() {
		unitPrice = lListPrice
	} else if matchedVariant != nil && matchedVariant.Price.IsPositive() {
		unitPrice = matchedVariant.Price
	} else if discPct > 0 && qty > 0 && lTotalPrice.IsPositive() && lTotalPrice.Minor() == lUnitPrice.Minor()*int64(qty) {
		rate := 1.0 - (discPct / 100.0)
		if rate > 0.001 {
			unitPrice = money.FromMinor(int64(float64(lUnitPrice.Minor()) / rate))
		}
	}

	netPrice = unitPrice
	if qty > 0 && lTotalPrice.IsPositive() {
		netPrice = money.FromMinor(lTotalPrice.Minor() / int64(qty))
	} else if discPct > 0 {
		netPrice = unitPrice.ApplyPercent(10000 - int64(discPct*100))
	}

	if unitPrice.Minor() <= netPrice.Minor() && discPct > 0 {
		rate := 1.0 - (discPct / 100.0)
		if rate > 0.001 {
			unitPrice = money.FromMinor(int64(float64(netPrice.Minor())/rate + 0.5))
		}
	}
	return unitPrice, discPct, netPrice
}

func formatDiscountPercent(pct float64) string {
	if pct <= 0 {
		return "0%"
	}
	if pct == float64(int64(pct)) {
		return fmt.Sprintf("%d%%", int64(pct))
	}
	s := fmt.Sprintf("%.2f", pct)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + "%"
}
