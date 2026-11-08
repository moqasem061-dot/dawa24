package orgasoft_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider/orgasoft"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeProductBatchCamelCase(t *testing.T) {
	p := orgasoft.New()
	if p.Name() != "orgasoft" {
		t.Fatalf("Name() = %q", p.Name())
	}
	batch, err := p.DecodeProductBatch(readFixture(t, "orgasoft_products_camel.json"))
	if err != nil {
		t.Fatal(err)
	}
	if batch.ClientBatchID != "" {
		t.Errorf("ClientBatchID = %q, want empty (fixture has none)", batch.ClientBatchID)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(batch.Items))
	}
	item := batch.Items[0]
	if item.Code != "4001" {
		t.Errorf("Code = %q", item.Code)
	}
	if item.ArName != "منتج مياه 1" {
		t.Errorf("ArName = %q", item.ArName)
	}
	if item.PublicPrice == nil || item.PublicPrice.String() != "1305.46" {
		t.Errorf("PublicPrice = %v, want json.Number 1305.46", item.PublicPrice)
	}
	if item.Stock == nil || item.Stock.String() != "1976" {
		t.Errorf("Stock = %v, want 1976", item.Stock)
	}
	if item.Discount == nil || item.Discount.String() != "2" {
		t.Errorf("Discount = %v, want 2", item.Discount)
	}
	if item.Tax == nil || item.Tax.String() != "14" {
		t.Errorf("Tax = %v, want 14", item.Tax)
	}
	if item.IsQuota == nil || *item.IsQuota {
		t.Errorf("IsQuota = %v, want false", item.IsQuota)
	}
	if item.QuotaQty == nil || item.QuotaQty.String() != "0" {
		t.Errorf("QuotaQty = %v, want 0", item.QuotaQty)
	}
	if item.Barcode != "6221790408214" {
		t.Errorf("Barcode = %q", item.Barcode)
	}
	if item.Expiry != "2028-01-10" {
		t.Errorf("Expiry = %q", item.Expiry)
	}
}

func TestDecodeProductBatchPascalCase(t *testing.T) {
	p := orgasoft.New()
	batch, err := p.DecodeProductBatch(readFixture(t, "orgasoft_products_pascal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(batch.Items))
	}
	item := batch.Items[0]
	if item.Code != "ORGA-100" {
		t.Errorf("Code = %q", item.Code)
	}
	if item.ArName != "منتج اورجا سوفت" {
		t.Errorf("ArName = %q", item.ArName)
	}
	if item.PublicPrice == nil || item.PublicPrice.String() != "250" {
		t.Errorf("PublicPrice = %v, want json.Number 250", item.PublicPrice)
	}
	if item.Stock == nil || item.Stock.String() != "50" {
		t.Errorf("Stock = %v, want 50", item.Stock)
	}
	if item.Expiry != "" {
		t.Errorf("Expiry = %q, want empty (PascalCase sample omits it)", item.Expiry)
	}
}

func TestDecodeProductBatchInlineCases(t *testing.T) {
	p := orgasoft.New()
	body := []byte(`{
		"ClientBatchId": "batch-77",
		"Items": [
			{"externalProductCode": 12345, "arName": "رقمي", "publicPrice": "62.50", "stock": "3"},
			{"externalProductCode": "67890", "arName": "نصي", "unknownField": {"x": 1}, "publicPrice": 7}
		],
		"unknownWrapper": [1, 2, 3]
	}`)
	batch, err := p.DecodeProductBatch(body)
	if err != nil {
		t.Fatal(err)
	}
	if batch.ClientBatchID != "batch-77" {
		t.Errorf("ClientBatchID = %q, want batch-77", batch.ClientBatchID)
	}
	if len(batch.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(batch.Items))
	}
	first := batch.Items[0]
	if first.Code != "12345" {
		t.Errorf("numeric code = %q, want \"12345\"", first.Code)
	}
	if first.PublicPrice == nil || first.PublicPrice.String() != "62.50" {
		t.Errorf("string price = %v, want json.Number \"62.50\"", first.PublicPrice)
	}
	if first.Stock == nil || first.Stock.String() != "3" {
		t.Errorf("string stock = %v, want 3", first.Stock)
	}
	second := batch.Items[1]
	if second.Code != "67890" {
		t.Errorf("string code = %q", second.Code)
	}
	if second.PublicPrice == nil || second.PublicPrice.String() != "7" {
		t.Errorf("number price = %v, want 7", second.PublicPrice)
	}
	// Money fields are json.Number everywhere, never float64.
	raw, _ := json.Marshal(first)
	var probe map[string]any
	_ = json.Unmarshal(raw, &probe)
}

func TestDecodeAckSample3(t *testing.T) {
	p := orgasoft.New()
	entries, err := p.DecodeAck(readFixture(t, "orgasoft_order_ack_sample3.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].OrderNumber != "{{orderNumber}}" {
		t.Errorf("OrderNumber = %q", entries[0].OrderNumber)
	}
	if entries[0].ErpReference != "ORG-9921" {
		t.Errorf("ErpReference = %q", entries[0].ErpReference)
	}
}

func TestDecodeAckCanonicalOrders(t *testing.T) {
	p := orgasoft.New()
	entries, err := p.DecodeAck([]byte(`{"orders":[{"orderNumber":"10542","erpReference":"ORGA-77123"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].OrderNumber != "10542" || entries[0].ErpReference != "ORGA-77123" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestDecodeAckSingleObject(t *testing.T) {
	p := orgasoft.New()
	entries, err := p.DecodeAck([]byte(`{"order_number":"10543","orga_orderID":"ORG-5"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].OrderNumber != "10543" || entries[0].ErpReference != "ORG-5" {
		t.Fatalf("entries = %+v", entries)
	}
}
