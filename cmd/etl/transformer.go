package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SourceUser represents a user record from MariaDB.
type SourceUser struct {
	ID        int64
	Name      string
	Email     string
	Password  string
	Role      string
	Phone     string
	CreatedAt time.Time
}

// TargetUser represents a transformed user record for PostgreSQL.
type TargetUser struct {
	ID           int64
	PublicID     string
	Email        string
	PasswordHash string
	Name         i18n.Text
	Role         string
	Status       string
	Language     string
	Phone        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SourceOrg represents a supplier/organization from MariaDB.
type SourceOrg struct {
	ID                 int64
	Name               string
	TaxNumber          string
	CommercialRegister string
	Phone              string
	Type               string
	Status             string
	CreatedAt          time.Time
}

// TargetOrg represents an organization for PostgreSQL.
type TargetOrg struct {
	ID                 int64
	PublicID           string
	LegalName          i18n.Text
	TradeName          i18n.Text
	TaxNumber          string
	CommercialRegister string
	Phone              string
	Type               string
	Status             string
	CreditLimit        money.Amount
	PaymentTermsDays   int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SourceProduct represents a product from MariaDB.
type SourceProduct struct {
	ID          int64
	NameAr      string
	NameEn      string
	Slug        string
	Description string
	CategoryID  int64
	Price       float64
	Stock       int
	VendorOrgID int64
	CreatedAt   time.Time
}

// TargetProduct represents a transformed product for PostgreSQL.
type TargetProduct struct {
	ID                   int64
	PublicID             string
	CategoryID           int64
	Name                 i18n.Text
	Slug                 string
	Description          i18n.Text
	DosageForm           string
	RequiresPrescription bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// TargetVariant represents a transformed variant for PostgreSQL.
type TargetVariant struct {
	ID        int64
	PublicID  string
	ProductID int64
	SKU       string
	Price     money.Amount
	Stock     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Transformer handles data cleaning, Arabic normalization, and money conversions.
type Transformer struct{}

func NewTransformer() *Transformer {
	return &Transformer{}
}

func (t *Transformer) TransformUser(src *SourceUser) *TargetUser {
	cleanEmail := strings.ToLower(strings.TrimSpace(src.Email))
	normName := arabic.Normalize(src.Name)

	return &TargetUser{
		ID:           src.ID,
		PublicID:     fmt.Sprintf("usr_%d", src.ID),
		Email:        cleanEmail,
		PasswordHash: src.Password, // Laravel bcrypt hashes ($2y$) are 100% compatible with Go bcrypt
		Name: i18n.Text{
			"ar": normName,
			"en": src.Name,
		},
		Role:      LegacyUserRole(src.Role),
		Status:    "active",
		Language:  "ar",
		Phone:     cleanPhone(src.Phone),
		CreatedAt: src.CreatedAt.UTC(),
		UpdatedAt: src.CreatedAt.UTC(),
	}
}

// LegacyOrgType maps a MariaDB organizations.type value onto one of the two
// account types (Rebuild V2 rule 1). Legacy: supplier/company/agency are
// vendors; pharmacy/chain_pharmacy/individual are customers. This answers the
// question 034 left open: company and agency are suppliers.
func LegacyOrgType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "vendor", "supplier", "company", "agency":
		return "vendor"
	default:
		return "customer"
	}
}

// LegacyUserRole maps a legacy users.role onto the platform-role vocabulary
// migration 060 enforces. Everything non-staff becomes 'user'; the legacy
// vendor/customer/pharmacy/individual roles expressed capability, which now
// comes from the organization membership.
func LegacyUserRole(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "super_admin", "admin", "support", "developer":
		return strings.ToLower(strings.TrimSpace(r))
	default:
		return "user"
	}
}

func (t *Transformer) TransformOrg(src *SourceOrg) *TargetOrg {
	normName := arabic.Normalize(src.Name)
	return &TargetOrg{
		ID:       src.ID,
		PublicID: fmt.Sprintf("org_%d", src.ID),
		LegalName: i18n.Text{
			"ar": normName,
			"en": src.Name,
		},
		TradeName: i18n.Text{
			"ar": normName,
			"en": src.Name,
		},
		TaxNumber:          src.TaxNumber,
		CommercialRegister: src.CommercialRegister,
		Phone:              cleanPhone(src.Phone),
		Type:               LegacyOrgType(src.Type),
		Status:             "approved",
		CreditLimit:        money.Zero,
		PaymentTermsDays:   30,
		CreatedAt:          src.CreatedAt.UTC(),
		UpdatedAt:          src.CreatedAt.UTC(),
	}
}

func (t *Transformer) TransformProduct(src *SourceProduct) (*TargetProduct, *TargetVariant) {
	normAr := arabic.Normalize(src.NameAr)
	amount, _ := money.Parse(fmt.Sprintf("%.2f", src.Price))

	p := &TargetProduct{
		ID:         src.ID,
		PublicID:   fmt.Sprintf("prd_%d", src.ID),
		CategoryID: src.CategoryID,
		Name: i18n.Text{
			"ar": normAr,
			"en": src.NameEn,
		},
		Slug: src.Slug,
		Description: i18n.Text{
			"ar": src.Description,
			"en": src.Description,
		},
		DosageForm:           "tablet",
		RequiresPrescription: false,
		CreatedAt:            src.CreatedAt.UTC(),
		UpdatedAt:            src.CreatedAt.UTC(),
	}

	v := &TargetVariant{
		ID:        src.ID,
		PublicID:  fmt.Sprintf("var_%d", src.ID),
		ProductID: src.ID,
		SKU:       src.Slug + "-DEFAULT",
		Price:     amount,
		Stock:     src.Stock,
		CreatedAt: src.CreatedAt.UTC(),
		UpdatedAt: src.CreatedAt.UTC(),
	}

	return p, v
}

func cleanPhone(phone string) string {
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	return cleaned
}
