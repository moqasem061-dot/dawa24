package datasets

import (
	"fmt"
	"sync"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

// SQL fragments shared by the declarations. They are Go constants formatted
// with alias names that are themselves constants in the declarations, so no
// part of any statement is built from a request.

// localized reads a {"ar","en"} JSONB name, Arabic first, then a bare string.
func localized(col string) string {
	return fmt.Sprintf(`COALESCE(NULLIF(%[1]s->>'ar',''), NULLIF(%[1]s->>'en',''), NULLIF(%[1]s#>>'{}',''), '')`, col)
}

// orgName is an organisation's display name.
func orgName(alias string) string {
	return fmt.Sprintf(`COALESCE(NULLIF(%[1]s.trade_name->>'ar',''), NULLIF(%[1]s.trade_name->>'en',''), NULLIF(%[2]s,''), NULLIF(%[1]s.legal_name,''), '')`,
		alias, localized(alias+".name"))
}

// personName is a user's display name.
func personName(alias string) string {
	return fmt.Sprintf(`COALESCE(NULLIF(trim(concat_ws(' ', %[1]s.first_name, %[1]s.last_name)),''), NULLIF(%[2]s,''), '')`,
		alias, localized(alias+".name"))
}

// Handle kinds the dataset engine introduces beyond the existing read tools.
const (
	KindCartLine handles.Kind = "crt"
	KindStock    handles.Kind = "stk"
	KindQuote    handles.Kind = "quo"
	KindIssue    handles.Kind = "iss"
)

var (
	defaultOnce sync.Once
	defaultCat  *Catalog
)

// Default is the process-wide catalogue: every dataset for every dashboard.
func Default() *Catalog {
	defaultOnce.Do(func() {
		var all []Dataset
		for _, scope := range []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor} {
			all = append(all, buyingDatasets(scope)...)
			all = append(all, companyDatasets(scope)...)
		}
		all = append(all, vendorDatasets()...)
		all = append(all, adminDatasets()...)
		defaultCat = New(all...)
	})
	return defaultCat
}

// Field constructors keep declarations readable.

func text(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Text}
}
func search(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Text, Search: true}
}
func money(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Money}
}
func integer(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Int}
}
func number(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Number}
}
func flag(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Bool}
}
func day(name, label, sql string) Field { return Field{Name: name, Label: label, SQL: sql, Type: Date} }
func instant(name, label, sql string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Time}
}

func enum(name, label, sql string, values ...string) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Enum, Values: values}
}

func ref(name, label, sql string, kind handles.Kind) Field {
	return Field{Name: name, Label: label, SQL: sql, Type: Ref, RefKind: kind, Hidden: true}
}

func hidden(f Field) Field { f.Hidden = true; return f }

func needs(f Field, perms ...string) Field { f.Permissions = perms; return f }

// Dashboard page kinds a record can link to; the assistant package resolves
// them to URLs per dashboard.
const (
	entityOrder        = "order"
	entityShipment     = "shipment"
	entityProduct      = "product"
	entityOffer        = "offer"
	entityOrganization = "organization"
	entityBranch       = "branch"
)

func key(sql string, kind handles.Kind, entity, label string) *Key {
	return &Key{SQL: sql, Kind: kind, Entity: entity, LabelField: label}
}

// Known status vocabularies, as stored.
var (
	orderStatuses    = []string{"pending", "processing", "confirmed", "on_hold", "shipped", "in_transit", "out_for_delivery", "delivered", "completed", "cancelled", "failed", "returned", "refunded"}
	shipmentStatuses = []string{"pending", "confirmed", "out_for_delivery", "delivered", "failed", "cancelled"}
	paymentStatuses  = []string{"unpaid", "paid"}
	paymentMethods   = []string{"cash", "cod", "wallet"}
	invoiceStatuses  = []string{"issued", "partially_paid", "paid"}
	walletTxTypes    = []string{"deposit", "purchase", "refund", "withdrawal"}
	requestStatuses  = []string{"pending", "approved", "processing", "completed", "cancelled"}
	quoteStatuses    = []string{"pending", "quoted", "accepted", "rejected", "expired"}
)
