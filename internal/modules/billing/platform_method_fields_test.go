package billing

import "testing"

// The wallet top-up modal shows only the credentials of the chosen method.
func TestScopeFieldsToProvider(t *testing.T) {
	full := func(provider string) *PlatformPaymentMethod {
		return &PlatformPaymentMethod{
			ProviderType: provider, AccountName: "holder", BankName: "bank", AccountNumber: "123",
			IBAN: "EG00", SwiftCode: "SW", BranchName: "br", InstaPayHandle: "x@instapay", PhoneNumber: "010",
		}
	}
	tests := []struct {
		provider                     string
		wantBank, wantIPA, wantPhone bool
	}{
		{"bank", true, false, false},
		{"instapay", false, true, true},
		{"wallet", false, false, true},
		{"card", false, false, false},
		{"cash", false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			p := full(tc.provider)
			p.ScopeFieldsToProvider()
			bank := p.AccountName != "" && p.BankName != "" && p.AccountNumber != "" && p.IBAN != "" && p.SwiftCode != "" && p.BranchName != ""
			noBank := p.AccountName == "" && p.BankName == "" && p.AccountNumber == "" && p.IBAN == "" && p.SwiftCode == "" && p.BranchName == ""
			if tc.wantBank && !bank || !tc.wantBank && !noBank {
				t.Errorf("bank fields kept=%v, want %v", !noBank, tc.wantBank)
			}
			if (p.InstaPayHandle != "") != tc.wantIPA {
				t.Errorf("instapay handle kept=%v, want %v", p.InstaPayHandle != "", tc.wantIPA)
			}
			if (p.PhoneNumber != "") != tc.wantPhone {
				t.Errorf("phone kept=%v, want %v", p.PhoneNumber != "", tc.wantPhone)
			}
		})
	}
}
