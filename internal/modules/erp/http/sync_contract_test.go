package http

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// These tests pin the product-sync read replies to contracts/openapi.yaml
// (SyncStatus, SyncItem, Page). The contract is what the ERP partner builds
// against, so the exact key set is part of the API: no internal field may
// leak and no documented field may go missing.

func keysOf(m map[string]any) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func decodeSyncEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, body)
	}
	return env
}

func TestSyncStatusMatchesContract(t *testing.T) {
	h := syncHandlerFixture(t)
	w := h.get("/products/sync/" + h.syncID(t))
	if w.Code != 200 {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	env := decodeSyncEnvelope(t, w.Body.Bytes())
	data, _ := env["data"].(map[string]any)
	if got, want := keysOf(data), "acceptedAt,clientBatchId,counts,finishedAt,itemCount,startedAt,state,syncId"; got != want {
		t.Fatalf("data keys = %s, want %s", got, want)
	}
	counts, _ := data["counts"].(map[string]any)
	if got, want := keysOf(counts), "created,failed,needsReview,unchanged,updated"; got != want {
		t.Fatalf("counts keys = %s, want %s", got, want)
	}
}

func TestSyncItemsMatchContract(t *testing.T) {
	h := syncHandlerFixture(t)
	w := h.get("/products/sync/" + h.syncID(t) + "/items")
	if w.Code != 200 {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	env := decodeSyncEnvelope(t, w.Body.Bytes())
	if got, want := keysOf(env), "data,page,requestId,status"; got != want {
		t.Fatalf("envelope keys = %s, want %s", got, want)
	}
	items, ok := env["data"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("data is not a non-empty array: %s", w.Body.String())
	}
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if got, want := keysOf(item), "code,externalProductCode,message,position,state"; got != want {
			t.Fatalf("item keys = %s, want %s", got, want)
		}
	}
	page, _ := env["page"].(map[string]any)
	if got, want := keysOf(page), "limit,nextCursor"; got != want {
		t.Fatalf("page keys = %s, want %s", got, want)
	}
	if page["limit"] != float64(1000) {
		t.Errorf("default limit = %v, want 1000", page["limit"])
	}
}

func TestSyncItemsRefusesBadCursor(t *testing.T) {
	h := syncHandlerFixture(t)
	w := h.get("/products/sync/" + h.syncID(t) + "/items?cursor=not-a-cursor")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "ERP_CURSOR_INVALID") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
