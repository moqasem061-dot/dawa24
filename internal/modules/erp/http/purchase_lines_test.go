package http

import (
	"encoding/json"
	"testing"
)

func TestVariantIDText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"string id", `"91530"`, "91530", true},
		{"legacy uuid string", `"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33"`, "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33", true},
		{"integer", `91530`, "91530", true},
		{"missing", ``, "", true},
		{"null", `null`, "", true},
		{"fraction", `91530.5`, "", false},
		{"zero", `0`, "", false},
		{"negative", `-4`, "", false},
		{"boolean", `true`, "", false},
		{"object", `{"id":1}`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := variantIDText(json.RawMessage(tc.raw))
			if got != tc.want || ok != tc.ok {
				t.Fatalf("variantIDText(%s) = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestLineCode(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string
		present bool
		ok      bool
	}{
		{"none", `{}`, "", false, false},
		{"code string trimmed", `{"code":"  ORGA-100 "}`, "ORGA-100", true, true},
		{"code number", `{"code":100001}`, "100001", true, true},
		{"itemId alias", `{"itemId":"A-1"}`, "A-1", true, true},
		{"itemCode alias number", `{"itemCode":7}`, "7", true, true},
		{"code wins over alias", `{"code":"C","itemId":"I"}`, "C", true, true},
		{"blank after trim", `{"code":"   "}`, "", true, false},
		{"too long", `{"code":"` + repeat('x', 65) + `"}`, repeat('x', 65), true, false},
		{"exactly 64", `{"code":"` + repeat('x', 64) + `"}`, repeat('x', 64), true, true},
		{"object refused", `{"code":{"a":1}}`, "", true, false},
		{"array refused", `{"code":[1]}`, "", true, false},
		{"null is absent", `{"code":null,"itemId":"Z"}`, "Z", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var line purchaseLineBody
			if err := json.Unmarshal([]byte(tc.body), &line); err != nil {
				t.Fatal(err)
			}
			got, present, ok := lineCode(line)
			if got != tc.want || present != tc.present || ok != tc.ok {
				t.Fatalf("lineCode(%s) = %q, %v, %v; want %q, %v, %v", tc.body, got, present, ok, tc.want, tc.present, tc.ok)
			}
		})
	}
}

func repeat(c byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

// TestPurchaseBodyAcceptsItemsAndQty pins the partner's names: items/qty give
// the same request as the earlier lines/quantity, and a bad code names the
// list the caller used.
func TestPurchaseBodyAcceptsItemsAndQty(t *testing.T) {
	decode := func(raw string) purchaseBody {
		t.Helper()
		var b purchaseBody
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		return b
	}
	newer, bad := decode(`{"externalReference":"R-1","items":[{"variantId":91530,"qty":12,"code":100001}]}`).request()
	if bad != nil {
		t.Fatalf("items body refused: %v", bad)
	}
	older, bad := decode(`{"externalReference":"R-1","lines":[{"variantId":"91530","quantity":12,"code":"100001"}]}`).request()
	if bad != nil {
		t.Fatalf("lines body refused: %v", bad)
	}
	if len(newer.Lines) != 1 || newer.Lines[0].VariantID != "91530" || newer.Lines[0].Quantity != 12 || newer.Lines[0].Code != "100001" {
		t.Fatalf("items body = %+v, want variant 91530, qty 12, code 100001", newer.Lines)
	}
	if older.Lines[0] != newer.Lines[0] {
		t.Fatalf("lines body = %+v, items body = %+v; want the same line", older.Lines[0], newer.Lines[0])
	}
	_, bad = decode(`{"externalReference":"R-2","items":[{"variantId":"91530","qty":1,"code":""}]}`).request()
	if bad == nil || bad.Details["field"] != "items[1].code" {
		t.Fatalf("empty code = %+v, want ERP_VALIDATION on items[1].code", bad)
	}
}
