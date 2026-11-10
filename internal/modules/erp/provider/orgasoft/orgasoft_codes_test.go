package orgasoft_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider/orgasoft"
)

// A row with externalProductCode and an alias must always decode to the
// externalProductCode, whatever order the row's keys are visited in.
func TestItemCodeExternalProductCodeAlwaysWins(t *testing.T) {
	p := orgasoft.New()
	body := []byte(`{"items":[{"externalProductCode":"EPC-1","itemId":"ITEM-9","itemCode":"IC-5","productCode":"PC-7","code":"C-3"}]}`)
	for i := 0; i < 50; i++ {
		batch, err := p.DecodeProductBatch(body)
		if err != nil {
			t.Fatal(err)
		}
		if got := batch.Items[0].Code; got != "EPC-1" {
			t.Fatalf("run %d: Code = %q, want EPC-1", i, got)
		}
	}
}

func TestItemCodeAliasPriorityWithoutExternalCode(t *testing.T) {
	p := orgasoft.New()
	cases := map[string]struct{ body, want string }{
		"itemId beats itemCode":      {`{"items":[{"itemCode":"IC-5","itemId":"ITEM-9"}]}`, "ITEM-9"},
		"itemCode beats productCode": {`{"items":[{"productCode":"PC-7","itemCode":"IC-5"}]}`, "IC-5"},
		"productCode beats code":     {`{"items":[{"code":"C-3","productCode":"PC-7"}]}`, "PC-7"},
		"code alone":                 {`{"items":[{"code":"C-3"}]}`, "C-3"},
	}
	for name, tc := range cases {
		for i := 0; i < 20; i++ {
			batch, err := p.DecodeProductBatch([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if got := batch.Items[0].Code; got != tc.want {
				t.Fatalf("%s: run %d: Code = %q, want %q", name, i, got, tc.want)
			}
		}
	}
}

// Orgasoft sends ItemId as a JSON number in its feed; it must be kept as text.
func TestNumericItemIdBecomesItsCode(t *testing.T) {
	p := orgasoft.New()
	batch, err := p.DecodeProductBatch([]byte(`{"items":[{"ItemId": 100001, "arName": "سبيركودا"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := batch.Items[0].Code; got != "100001" {
		t.Fatalf("Code = %q, want 100001", got)
	}
}

// An ack names the order by orderId, a JSON number or a string.
func TestDecodeAckOrderIDNumberAndString(t *testing.T) {
	p := orgasoft.New()
	cases := map[string]string{
		"number": `[{"orderId":21845,"erpReference":"ORGA-77"}]`,
		"string": `[{"orderId":"21845","erpReference":"ORGA-77"}]`,
	}
	for name, body := range cases {
		entries, err := p.DecodeAck([]byte(body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(entries) != 1 || entries[0].OrderNumber != "21845" || entries[0].ErpReference != "ORGA-77" {
			t.Fatalf("%s: entries = %+v", name, entries)
		}
	}
}

func TestDecodeAckErpOrderIDReferenceKeys(t *testing.T) {
	p := orgasoft.New()
	for _, key := range []string{"erpOrderId", "orgaOrderId"} {
		entries, err := p.DecodeAck([]byte(`{"orderNumber":"DW-1","` + key + `":"ORGA-88"}`))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if entries[0].ErpReference != "ORGA-88" {
			t.Fatalf("%s: ErpReference = %q", key, entries[0].ErpReference)
		}
	}
}
