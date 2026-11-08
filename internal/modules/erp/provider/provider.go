// Package provider is the seam between the ERP module and the concrete ERP
// systems. The module knows providers only through the Provider interface and
// the registry; a wire spelling lives in the provider package that owns it
// (trap 28: no code outside the provider packages may name a provider).
package provider

import "encoding/json"

// Provider decodes one ERP system's wire format.
type Provider interface {
	// Name is the registry key (matches erp.integrations.provider).
	Name() string
	// DecodeProductBatch turns a product-sync body into raw items. It never
	// touches the database and never fails on unknown fields.
	DecodeProductBatch(body []byte) (Batch, error)
	// DecodeAck turns an order-acknowledgement body into references.
	DecodeAck(body []byte) ([]AckEntry, error)
}

// Batch is one decoded product-sync payload.
type Batch struct {
	// ClientBatchID is the ERP's own batch reference, optional.
	ClientBatchID string
	Items         []RawItem
}

// RawItem is one ERP item exactly as the wire carried it: every number stays a
// *json.Number (money must never become a float64 — trap 13) and the field
// names mirror the wire. Orgasoft sends no English name, and Unit,
// Manufacturer and Batch (wire name `batchNumber`) are optional fields that
// appear in no Orgasoft sample; they are unverified and ignored when absent.
type RawItem struct {
	Code         string
	ArName       string
	Barcode      string
	Unit         string
	Manufacturer string
	Batch        string
	Expiry       string
	// PublicPrice, Discount, Tax, Stock and QuotaQty keep the wire text so the
	// normaliser can validate exactness (two decimals) before any money math.
	PublicPrice *json.Number
	Discount    *json.Number
	Tax         *json.Number
	Stock       *json.Number
	QuotaQty    *json.Number
	IsQuota     *bool
}

// AckEntry is one order acknowledgement: the ERP's reference for one of our
// order numbers.
type AckEntry struct {
	OrderNumber  string
	ErpReference string
}
