package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestSmartOrderSamples(t *testing.T) {
	h := &UIHandler{}

	t.Run("CSV sample download", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/customer/smart-order/sample.csv", nil)
		rec := httptest.NewRecorder()
		h.SmartOrderSampleCSV(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/csv") {
			t.Errorf("content-type = %q, want text/csv", ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "اسم الصنف") {
			t.Errorf("body missing 'اسم الصنف': %s", body)
		}
		if !strings.Contains(body, "الكمية المطلوبة") {
			t.Errorf("body missing 'الكمية المطلوبة': %s", body)
		}
		if !strings.Contains(body, "CONG-TAB-20") {
			t.Errorf("body missing sample SKU CONG-TAB-20: %s", body)
		}
	})

	t.Run("XLSX sample download", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/customer/smart-order/sample.xlsx", nil)
		rec := httptest.NewRecorder()
		h.SmartOrderSampleXLSX(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.Contains(ct, "openxmlformats") {
			t.Errorf("content-type = %q, want spreadsheetml", ct)
		}

		f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("failed to parse generated Excel file: %v", err)
		}
		defer f.Close()

		rows, err := f.GetRows("نواقص الطلب الذكي")
		if err != nil {
			t.Fatalf("failed to get sheet rows: %v", err)
		}
		if len(rows) < 6 {
			t.Fatalf("expected at least 6 rows (header + 5 samples), got %d", len(rows))
		}
		if rows[0][0] != "اسم الصنف / الدواء" {
			t.Errorf("col 0 = %q, want 'اسم الصنف / الدواء'", rows[0][0])
		}
	})
}
