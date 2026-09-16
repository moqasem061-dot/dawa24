package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateInvoice generates a new B2B invoice.
func (s *Service) CreateInvoice(ctx context.Context, inv *Invoice) (*Invoice, error) {
	if inv.OrganizationID <= 0 {
		return nil, apperr.Validation("invoice.org_required", "Organization ID is required.", nil)
	}
	if inv.InvoiceNumber == "" {
		inv.InvoiceNumber = fmt.Sprintf("INV-%d-%d", inv.OrganizationID, time.Now().Unix())
	}
	if inv.Status == "" {
		inv.Status = InvoiceDraft
	}

	if inv.Subtotal.IsZero() {
		var gross money.Amount
		for _, l := range inv.Lines {
			lineGross, _ := l.UnitPrice.MulInt(int64(l.Quantity))
			gross, _ = gross.Add(lineGross)
		}
		if gross.IsZero() {
			for _, l := range inv.Lines {
				gross, _ = gross.Add(l.TotalPrice)
			}
			if inv.DiscountAmount.IsPositive() {
				gross, _ = gross.Add(inv.DiscountAmount)
			}
		}
		inv.Subtotal = gross
	}
	if inv.TotalAmount.IsZero() {
		total, _ := inv.Subtotal.Sub(inv.DiscountAmount)
		total, _ = total.Add(inv.TaxAmount)
		if total.IsNegative() {
			total = money.Zero
		}
		inv.TotalAmount = total
	}

	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "invoice created", "invoice_id", inv.ID, "number", inv.InvoiceNumber, "total", inv.TotalAmount.String())
	return inv, nil
}

// GetInvoice returns an invoice by ID.
func (s *Service) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	return s.repo.GetInvoiceByID(ctx, id)
}

// GetInvoiceByID returns an invoice by ID.
func (s *Service) GetInvoiceByID(ctx context.Context, id int64) (*Invoice, error) {
	return s.repo.GetInvoiceByID(ctx, id)
}

// GetInvoiceByOrderID returns an invoice by its associated order ID.
func (s *Service) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*Invoice, error) {
	return s.repo.GetInvoiceByOrderID(ctx, orderID)
}

// ListInvoices lists invoices for an organization.
func (s *Service) ListInvoices(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, error) {
	return s.repo.ListInvoicesByOrg(ctx, orgID, limit, offset)
}

// ListInvoicesWithTotal lists paginated invoices for an organization with total count.
func (s *Service) ListInvoicesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, int, error) {
	return s.repo.ListInvoicesByOrgWithTotal(ctx, orgID, limit, offset)
}

// MarkInvoicePaid updates invoice status to paid.
func (s *Service) MarkInvoicePaid(ctx context.Context, id int64) error {
	return s.repo.UpdateInvoiceStatus(ctx, id, InvoicePaid)
}
