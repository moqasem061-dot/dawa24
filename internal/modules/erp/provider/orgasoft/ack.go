package orgasoft

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// DecodeAck reads an order acknowledgement. Three shapes are accepted, because
// the Orgasoft sample (sample 3) is a bare array of objects while the agreed
// canonical body wraps them in "orders":
//   - {"orders":[{"orderNumber":"…","erpReference":"…"}]} (canonical)
//   - {"order_number":"…","orga_orderID":"…"} (one object, sample-style keys)
//   - [{"order_number":"…","orga_orderID":"…"}] (bare array, sample 3)
//
// The embedded per-item "items" array of sample 3 is ignored: the
// acknowledgement carries only the two reference strings.
func (o *Orgasoft) DecodeAck(body []byte) ([]provider.AckEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var top any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("orgasoft: decode ack: %w", err)
	}
	switch wrapped := top.(type) {
	case map[string]any:
		if orders, ok := wrapped["orders"]; ok {
			return ackFromSlice(orders)
		}
		entry, err := ackEntry(wrapped)
		if err != nil {
			return nil, err
		}
		return []provider.AckEntry{entry}, nil
	case []any:
		return ackFromSlice(wrapped)
	default:
		return nil, fmt.Errorf("orgasoft: ack body is neither an object nor an array")
	}
}

func ackFromSlice(raw any) ([]provider.AckEntry, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("orgasoft: orders is not an array")
	}
	entries := make([]provider.AckEntry, 0, len(list))
	for _, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("orgasoft: ack entry is not an object")
		}
		entry, err := ackEntry(obj)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ackEntry reads one acknowledgement object. Both the canonical camelCase keys
// and the sample's snake_case `order_number` / `orga_orderID` carry the pair;
// unknown keys (the sample's `items`) are ignored (research D-18).
func ackEntry(obj map[string]any) (provider.AckEntry, error) {
	entry := provider.AckEntry{}
	// orderId is the id Orgasoft read in the order feed; it arrives as a JSON
	// number or a string and is matched exactly as sent.
	orderNumber, err := ackString(obj, "orderNumber", "order_number", "orderId")
	if err != nil {
		return entry, err
	}
	reference, err := ackString(obj, "erpReference", "orga_orderID", "erpOrderId", "orgaOrderId")
	if err != nil {
		return entry, err
	}
	entry.OrderNumber = orderNumber
	entry.ErpReference = reference
	return entry, nil
}

func ackString(obj map[string]any, keys ...string) (string, error) {
	for _, key := range keys {
		value, ok := obj[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed, nil
		case json.Number:
			return typed.String(), nil
		default:
			return "", fmt.Errorf("orgasoft: ack key %s is not a string or number", key)
		}
	}
	return "", fmt.Errorf("orgasoft: ack object is missing %v", keys)
}
