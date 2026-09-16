package pages

import (
	"fmt"
)

// RegisterFormData carries the registration form state so a validation failure
// re-renders the form with the user's entered values still in the fields.
// A registration form that empties itself on error is the fastest way to lose a
// signup, so every field is echoed back.
type RegisterFormData struct {
	AccountType        string
	Name               string
	Email              string
	Phone              string
	LegalName          string
	TradeNameAr        string
	TradeNameEn        string
	CommercialRegister string
	TaxNumber          string
	PharmacistLicense  string
	LicenseDocumentURL string
	GovernorateID      string
	CityID             string
	BranchCount        string
	Address            string
	Latitude           string
	Longitude          string
	GoogleMapsURL      string
	Specialisation     string
	YearsExperience    string
	Bio                string
	ExpectedSalary     string
	CVStorageKey       string
	VerifiedPhoneToken string
	PhoneVerified      bool
	Error              string
}

// AlpineState renders the Alpine.js x-data payload for the two-step form.
// If an account type is already present (a failed submit re-render), the form
// opens on step 2 with that type selected so the user's work is not lost.
func (f RegisterFormData) AlpineState() string {
	step := 1
	if f.AccountType != "" {
		step = 2
	}
	return fmt.Sprintf("{ step: %d, accountType: %q }", step, f.AccountType)
}

// CitySelected reported whether a <select> option was the chosen city. The
// pickers are comboboxes now and carry their selection as a value and a label,
// so there is nothing to compare per option.
