package billing

// ScopeFieldsToProvider clears every credential that does not belong to the
// method's provider type.
//
// The admin form only hides the inputs of the other provider types, so their
// values were still submitted and stored: an InstaPay channel carried a bank
// name and account number, and the wallet top-up modal showed them all as if
// they were InstaPay credentials. Applied on save and again before display, so
// rows stored before the fix render correctly too.
func (p *PlatformPaymentMethod) ScopeFieldsToProvider() {
	if p == nil {
		return
	}
	keepBank, keepInstaPay, keepPhone := false, false, false
	switch p.ProviderType {
	case "bank":
		keepBank = true
	case "instapay":
		keepInstaPay, keepPhone = true, true
	case "wallet":
		keepPhone = true
	}
	if !keepBank {
		p.AccountName, p.BankName, p.AccountNumber = "", "", ""
		p.IBAN, p.SwiftCode, p.BranchName = "", "", ""
	}
	if !keepInstaPay {
		p.InstaPayHandle = ""
	}
	if !keepPhone {
		p.PhoneNumber = ""
	}
}
