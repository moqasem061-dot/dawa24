package commerce

// shipmentBranchFor falls back to the offer-level branch when no per-vendor branch is resolved.
func shipmentBranchFor(input CheckoutInput, vendorOrgID int64) *int64 {
	if branch, ok := input.VendorBranchIDs[vendorOrgID]; ok && branch != nil && *branch > 0 {
		return branch
	}
	return input.VendorBranchID
}
