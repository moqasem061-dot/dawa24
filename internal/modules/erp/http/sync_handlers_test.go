package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

// The sync handler tests run the real handlers over an in-memory repository
// subset: the POST exercises AcceptBatch (scope + decode), the two GETs read
// GetBatchByPublicID and ListSyncItems, and the principal rides the context
// through the same key the auth middleware uses.

type syncHandlerRepo struct {
	erp.Repository
	mu                 sync.Mutex
	batches            map[string]*storedBatch
	items              map[string][]*storedItem
	keys               map[string]erp.KeyAuthRow
	orgStatus, orgType string
}

func (r *syncHandlerRepo) FindKeyByPrefixAsSystem(_ context.Context, prefix string) (erp.KeyAuthRow, bool, error) {
	row, ok := r.keys[prefix]
	return row, ok, nil
}

func (r *syncHandlerRepo) OrgStatusType(context.Context, int64) (string, string, error) {
	return r.orgStatus, r.orgType, nil
}

func (r *syncHandlerRepo) TouchLastUsed(context.Context, int64, int64, string) error { return nil }

// enqueuerStub accepts the chunk jobs the acceptance inserts.
type enqueuerStub struct{}

func (enqueuerStub) EnqueueTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return &rivertype.JobInsertResult{}, nil
}

type storedBatch struct {
	org       int64
	state     string
	itemCount int
}

type storedItem struct {
	externalCode string
	state        string
}

func newSyncHandlerRepo() *syncHandlerRepo {
	return &syncHandlerRepo{batches: map[string]*storedBatch{}, items: map[string][]*storedItem{}}
}

func (r *syncHandlerRepo) InsertBatch(_ context.Context, orgID int64, b erp.NewBatch, items []erp.NewItemRow, enqueue func(ctx context.Context, tx pgx.Tx, batch erp.SyncBatch) error) (erp.SyncBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := int64(len(r.batches) + 1)
	publicID := fmt.Sprintf("sync-%d", id)
	r.batches[publicID] = &storedBatch{org: orgID, state: "queued", itemCount: b.ItemCount}
	for _, row := range items {
		r.items[fmt.Sprintf("%d", id)] = append(r.items[fmt.Sprintf("%d", id)], &storedItem{externalCode: row.ExternalCode, state: row.State})
	}
	if enqueue != nil {
		if err := enqueue(context.Background(), nil, erp.SyncBatch{ID: id, PublicID: publicID, ChunkCount: b.ChunkCount}); err != nil {
			return erp.SyncBatch{}, err
		}
	}
	return erp.SyncBatch{ID: id, PublicID: publicID, OrganizationID: orgID, State: "queued", ItemCount: len(items), ChunkCount: b.ChunkCount, AcceptedAt: time.Now().UTC(), Provider: "orgasoft"}, nil
}

func (r *syncHandlerRepo) FindBatchByIdempotency(context.Context, int64, int64, string) (erp.SyncBatch, bool, error) {
	return erp.SyncBatch{}, false, nil
}

func (r *syncHandlerRepo) GetBatchByPublicID(_ context.Context, orgID int64, publicID string) (erp.SyncBatch, error) {
	store, ok := r.batches[publicID]
	if !ok || store.org != orgID {
		return erp.SyncBatch{}, erp.ErrNotFound
	}
	return erp.SyncBatch{ID: 1, PublicID: publicID, OrganizationID: orgID, State: store.state, ItemCount: store.itemCount, Provider: "orgasoft"}, nil
}

func (r *syncHandlerRepo) ListSyncItems(_ context.Context, orgID, batchID, afterID int64, state string, limit int) ([]erp.SyncItemRow, error) {
	var out []erp.SyncItemRow
	for _, item := range r.items[fmt.Sprintf("%d", batchID)] {
		if state != "" && item.state != state {
			continue
		}
		out = append(out, erp.SyncItemRow{ID: 1, BatchID: batchID, ExternalCode: item.externalCode, State: item.state})
	}
	return out, nil
}

// erpItem builds one camelCase wire item.
func erpItem(code, price string, stock int) map[string]any {
	return map[string]any{
		"externalProductCode": code,
		"arName":              "\u0635\u0646\u0641 " + code,
		"publicPrice":         price,
		"stock":               stock,
		"isQuota":             false,
		"quotaQuantity":       0,
	}
}

// ---- fixture ---------------------------------------------------------------

type syncFixture struct {
	t       *testing.T
	routes  chi.Router
	fullKey string
	repo    *syncHandlerRepo
}

func syncHandlerFixture(t *testing.T) *syncFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pepper := []byte("sync-handler-pepper-32-bytes")
	repo := newSyncHandlerRepo()
	repo.keys = map[string]erp.KeyAuthRow{}
	repo.orgStatus, repo.orgType = "approved", "vendor"
	svc := erp.NewService(repo, erp.Ports{Enqueuer: enqueuerStub{}}, erp.Config{
		Pepper: pepper, Tag: "test",
		DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider,
	}, logger)
	full, prefix, hash, err := erp.GenerateKey("test", pepper)
	if err != nil {
		t.Fatal(err)
	}
	repo.keys = map[string]erp.KeyAuthRow{prefix: {
		APIKey: erp.APIKey{ID: 1, OrganizationID: 7, IntegrationID: 3, KeyPrefix: prefix,
			Status: "active", Scopes: []string{"erp.products.sync", "erp.orders.read"},
			WarehouseID: 40, BranchID: 41, RateLimitPerMinute: 600, MaxBatchItems: 50000},
		KeyHash: hash, IntegrationStatus: "active", Provider: provider.DefaultProvider,
	}}
	repo.orgStatus, repo.orgType = "approved", "vendor"
	return &syncFixture{t: t, routes: NewHandler(svc, erp.Config{}, nil, logger).Routes(), fullKey: full, repo: repo}
}

func (h *syncFixture) do(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.fullKey)
	w := httptest.NewRecorder()
	h.routes.ServeHTTP(w, req)
	return w
}

func (h *syncFixture) post(path, body string) *httptest.ResponseRecorder {
	return h.do("POST", path, body)
}

func (h *syncFixture) get(path string) *httptest.ResponseRecorder {
	return h.do("GET", path, "")
}

func (h *syncFixture) syncID(t *testing.T) string {
	t.Helper()
	w := h.post("/products/sync", string(erpSyncBody(erpItem("NF1", "9.00", 1), erpItem("BAD", "nope", 1))))
	var env struct {
		Data struct {
			SyncID string `json:"syncId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.Data.SyncID
}

func erpSyncBody(items ...map[string]any) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"items":[`)
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		data, _ := json.Marshal(item)
		buf.Write(data)
	}
	buf.WriteString(`]}`)
	return buf.Bytes()
}

func TestPostSyncReturnsAccepted(t *testing.T) {
	h := syncHandlerFixture(t)
	w := h.post("/products/sync", string(erpSyncBody(erpItem("N1", "5.00", 1))))
	if w.Code != 202 {
		t.Fatalf("status = %d, want 202 (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"state":"queued"`) || !strings.Contains(w.Body.String(), "syncId") || !strings.Contains(w.Body.String(), `"statusUrl"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestSyncScopeEnforced(t *testing.T) {
	h := syncHandlerFixture(t)
	// The key's scopes are what AcceptBatch checks; a read-only key gets the
	// scope refusal.
	for prefix, row := range h.repo.keys {
		row.Scopes = []string{"erp.orders.read"}
		h.repo.keys[prefix] = row
	}
	w := h.post("/products/sync", string(erpSyncBody(erpItem("N2", "5.00", 1))))
	if w.Code != 403 || !strings.Contains(w.Body.String(), "ERP_SCOPE_MISSING") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ar"`) || !strings.Contains(w.Body.String(), `"en"`) {
		t.Error("error body is not bilingual")
	}
}

func TestGetSyncReturnsCounts(t *testing.T) {
	h := syncHandlerFixture(t)
	syncID := h.syncID(t)
	w := h.get("/products/sync/" + syncID)
	if w.Code != 200 {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"itemCount":2`) || !strings.Contains(w.Body.String(), `"state":"queued"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestGetSyncForeignIsNotFound(t *testing.T) {
	h := syncHandlerFixture(t)
	syncID := h.syncID(t)
	// Read through a different organization's tenant context: the same
	// not-found as a missing sync (US1.3).
	for prefix, row := range h.repo.keys {
		row.OrganizationID = 999
		h.repo.keys[prefix] = row
	}
	w := h.get("/products/sync/" + syncID)
	if w.Code != 404 || !strings.Contains(w.Body.String(), "ERP_SYNC_NOT_FOUND") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ar"`) || !strings.Contains(w.Body.String(), `"en"`) {
		t.Error("error body is not bilingual")
	}
}

func TestGetSyncItemsPagesAndFilters(t *testing.T) {
	h := syncHandlerFixture(t)
	syncID := h.syncID(t)
	w := h.get("/products/sync/" + syncID + "/items?state=failed")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"state":"failed"`) {
		t.Errorf("state filter ignored: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "BAD") {
		t.Errorf("the failed item's code missing: %s", w.Body.String())
	}
}
