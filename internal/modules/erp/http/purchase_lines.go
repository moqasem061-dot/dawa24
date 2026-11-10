package http

import (
	"encoding/json"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// maxLineCode is the longest item code a purchase line may carry.
const maxLineCode = 64

// variantIDText reads a purchase line's variantId. A JSON string is taken as sent,
// and a JSON integer becomes its decimal text. A missing or null value is "" (the
// service then refuses it as not found). Any other value is not ok.
func variantIDText(raw json.RawMessage) (string, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "", true
	}
	if strings.HasPrefix(s, `"`) {
		var out string
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", false
		}
		return out, true
	}
	if _, ok := erp.ParseShortID(s); ok {
		return s, true
	}
	return "", false
}

// lineCode picks a purchase line's item code from code, itemId or itemCode, in
// that order. present is false when none of them is sent. ok is false when the
// value is neither a string nor a number, or is empty or longer than 64
// characters once trimmed.
func lineCode(line purchaseLineBody) (code string, present, ok bool) {
	for _, raw := range []json.RawMessage{line.Code, line.ItemID, line.ItemCode} {
		s := strings.TrimSpace(string(raw))
		if s == "" || s == "null" {
			continue
		}
		text, valid := scalarText(raw)
		text = strings.TrimSpace(text)
		return text, true, valid && len(text) >= 1 && len(text) <= maxLineCode
	}
	return "", false, false
}

// scalarText reads a JSON string or a JSON number as text. Any other JSON value
// (object, array, boolean) is not valid.
func scalarText(raw json.RawMessage) (string, bool) {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, `"`) {
		var out string
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", false
		}
		return out, true
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return "", false
	}
	return num.String(), true
}
