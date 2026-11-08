// Package orgasoft decodes the Orgasoft ERP wire format: the camelCase product
// sample, the PascalCase sample dated 11 Mar 2026, and the order
// acknowledgement sample. Field spellings live in fields.go and nowhere else
// (trap 28).
package orgasoft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// Orgasoft is the provider for the Orgasoft ERP.
type Orgasoft struct{}

// New builds the provider. Registered in init (see providers.go's blank
// import of provider/all).
func New() *Orgasoft { return &Orgasoft{} }

func init() { provider.Register(New()) }

// Name is the registry key stored in erp.integrations.provider.
func (o *Orgasoft) Name() string { return provider.DefaultProvider }

// number turns a raw JSON value into a json.Number whether the wire sent a
// number or a numeric string. Anything else leaves the field nil; the item
// normaliser then reports the precise error.
func number(raw json.RawMessage) (*json.Number, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		if s == "" {
			return nil, nil
		}
		n := json.Number(s)
		return &n, nil
	}
	var n json.Number
	if err := json.Unmarshal(trimmed, &n); err != nil {
		return nil, fmt.Errorf("invalid number %s: %w", trimmed, err)
	}
	return &n, nil
}

// str accepts a JSON string, or a bare number (Orgasoft sends item codes as
// numbers in the PascalCase era samples).
func str(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	var n json.Number
	if err := json.Unmarshal(trimmed, &n); err != nil {
		return "", fmt.Errorf("invalid value %s: %w", trimmed, err)
	}
	return n.String(), nil
}

func boolean(raw json.RawMessage) (*bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	b, err := strconv.ParseBool(string(trimmed))
	if err != nil {
		var s string
		if jsonErr := json.Unmarshal(trimmed, &s); jsonErr == nil {
			if parsed, parseErr := strconv.ParseBool(s); parseErr == nil {
				return &parsed, nil
			}
		}
		return nil, fmt.Errorf("invalid boolean %s: %w", trimmed, err)
	}
	return &b, nil
}

// DecodeProductBatch reads one product-sync payload: whatever key folds to
// "items" carries the items; unknown keys at every level are ignored; every
// money value stays a json.Number.
func (o *Orgasoft) DecodeProductBatch(body []byte) (provider.Batch, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return provider.Batch{}, fmt.Errorf("orgasoft: decode batch: %w", err)
	}
	var batch provider.Batch
	for key, raw := range top {
		if foldKey(key) == "clientBatchId" { // camelCase and PascalCase both fold onto it
			id, err := str(raw)
			if err != nil {
				return provider.Batch{}, fmt.Errorf("orgasoft: clientBatchId: %w", err)
			}
			batch.ClientBatchID = id
			break
		}
	}
	var itemsRaw json.RawMessage
	for key, raw := range top {
		if foldKey(key) == "items" { // camelCase "items" and PascalCase "Items" both fold onto it
			itemsRaw = raw
			break
		}
	}
	if len(itemsRaw) == 0 {
		return batch, nil
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(itemsRaw, &rows); err != nil {
		return provider.Batch{}, fmt.Errorf("orgasoft: decode items: %w", err)
	}
	batch.Items = make([]provider.RawItem, 0, len(rows))
	// Folded lookup: variant spelling -> canonical wire name.
	lookup := make(map[string]string, len(productAliases))
	for canonical, spellings := range productAliases {
		for _, spelling := range spellings {
			lookup[foldKey(spelling)] = canonical
		}
	}
	for _, row := range rows {
		var item provider.RawItem
		for key, raw := range row {
			canonical, known := lookup[foldKey(key)]
			if !known {
				continue // unknown fields ignored
			}
			var err error
			switch canonical {
			case "externalProductCode":
				item.Code, err = str(raw)
			case "arName":
				item.ArName, err = str(raw)
			case "barcode":
				item.Barcode, err = str(raw)
			case "unit":
				item.Unit, err = str(raw)
			case "manufacturer":
				item.Manufacturer, err = str(raw)
			case "batchNumber":
				item.Batch, err = str(raw)
			case "expiryDate":
				item.Expiry, err = str(raw)
			case "publicPrice":
				item.PublicPrice, err = number(raw)
			case "discount":
				item.Discount, err = number(raw)
			case "tax":
				item.Tax, err = number(raw)
			case "stock":
				item.Stock, err = number(raw)
			case "quotaQuantity":
				item.QuotaQty, err = number(raw)
			case "isQuota":
				item.IsQuota, err = boolean(raw)
			}
			if err != nil {
				return provider.Batch{}, fmt.Errorf("orgasoft: field %s: %w", canonical, err)
			}
		}
		batch.Items = append(batch.Items, item)
	}
	return batch, nil
}
