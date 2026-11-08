package erp

import (
	"context"
)

// CodeRepository is the customer-code part of Repository (S5). The Postgres
// implementation lives in postgres/customer_code.go.
type CodeRepository interface {
	// SupplierERPStatusAsSystem is plan.md E8: for each supplier, whether it has
	// an active integration with an active, unrevoked, unexpired key. Booleans only.
	SupplierERPStatusAsSystem(ctx context.Context, vendorOrgIDs []int64) (map[int64]bool, error)
	// CodedSuppliers returns the suppliers for which this buyer has a pending or
	// approved link with a non-blank organization number.
	CodedSuppliers(ctx context.Context, buyerOrgID, buyerUserID int64, vendorOrgIDs []int64) (map[int64]bool, error)
	// GuardCustomerCode takes the supplier's advisory lock, refuses (taken=true)
	// a code another customer holds, and otherwise runs write in the same
	// transaction. The lock is held until that transaction commits.
	GuardCustomerCode(ctx context.Context, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64, write func(context.Context) error) (taken bool, err error)
}

// SuppliersUsingERP reports, per supplier, whether it has an active integration
// with an active key (plan.md E8). A supplier with no integration is absent.
func (s *Service) SuppliersUsingERP(ctx context.Context, vendorOrgIDs []int64) (map[int64]bool, error) {
	if len(vendorOrgIDs) == 0 {
		return map[int64]bool{}, nil
	}
	return s.repo.SupplierERPStatusAsSystem(ctx, vendorOrgIDs)
}

// MissingCodes returns, in input order, the ERP suppliers for which this buyer
// has no customer code. Suppliers without ERP are never listed (SC-009).
func (s *Service) MissingCodes(ctx context.Context, buyerOrgID, buyerUserID int64, vendorOrgIDs []int64) ([]int64, error) {
	using, err := s.SuppliersUsingERP(ctx, vendorOrgIDs)
	if err != nil {
		return nil, err
	}
	var erpSuppliers []int64
	for _, id := range vendorOrgIDs {
		if using[id] {
			erpSuppliers = append(erpSuppliers, id)
		}
	}
	if len(erpSuppliers) == 0 {
		return nil, nil
	}
	coded, err := s.repo.CodedSuppliers(ctx, buyerOrgID, buyerUserID, erpSuppliers)
	if err != nil {
		return nil, err
	}
	var missing []int64
	for _, id := range erpSuppliers {
		if !coded[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// GuardCustomerCode returns ERP_CODE_TAKEN (409) when another customer holds
// code with this supplier. It does not write. Writers use GuardCustomerCodeWrite.
func (s *Service) GuardCustomerCode(ctx context.Context, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64) error {
	return s.GuardCustomerCodeWrite(ctx, vendorOrgID, customerOrgID, userID, code, excludeLinkID, func(context.Context) error { return nil })
}

// GuardCustomerCodeWrite checks the code and, when it is free, runs write in the
// same transaction under the supplier's advisory lock (S5 D2). Two saves of the
// same code cannot both pass. A supplier without ERP is never refused.
func (s *Service) GuardCustomerCodeWrite(ctx context.Context, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64, write func(context.Context) error) error {
	taken, err := s.repo.GuardCustomerCode(ctx, vendorOrgID, customerOrgID, userID, code, excludeLinkID, write)
	if err != nil {
		return err
	}
	if taken {
		return NewError("ERP_CODE_TAKEN", 409, nil)
	}
	return nil
}
