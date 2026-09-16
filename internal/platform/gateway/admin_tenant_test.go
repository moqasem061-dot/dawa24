package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A fake Gateway management surface, counting the calls that matter.
//
// The count of POST /api/keys is the whole point of these tests: issuing a key
// revokes the previous one, so a provisioning path that mints unnecessarily is
// not merely wasteful, it destroys a credential another caller is using.
type fakeGateway struct {
	keysIssued  atomic.Int32
	usersPosted atomic.Int32
	usersPut    atomic.Int32

	// existingKeys is what GET /api/keys reports.
	existingKeys []GatewayVirtualKey
	// acceptedKeys are the Bearer tokens /v1/models answers 200 for.
	acceptedKeys map[string]bool
	// userPostStatus lets a test make POST /api/users conflict, so the update
	// fallback is exercised.
	userPostStatus int
	// lastPlanID records the plan the last upsert asked for.
	lastPlanID atomic.Value
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{acceptedKeys: map[string]bool{}, userPostStatus: http.StatusCreated}
}

func (f *fakeGateway) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		var u GatewayUser
		_ = json.NewDecoder(r.Body).Decode(&u)
		f.lastPlanID.Store(u.PlanID)
		switch r.Method {
		case http.MethodPost:
			f.usersPosted.Add(1)
			w.WriteHeader(f.userPostStatus)
		case http.MethodPut:
			f.usersPut.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(f.existingKeys)
		case http.MethodPost:
			n := f.keysIssued.Add(1)
			key := GatewayVirtualKey{
				ID: "k", UserID: "org-7", Status: "active",
				Key: "sk-virt-minted-" + string(rune('0'+n)),
			}
			f.acceptedKeys[key.Key] = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(key)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !f.acceptedKeys[token] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestEnsureOrganizationReusesAValidStoredKey(t *testing.T) {
	f := newFakeGateway()
	f.acceptedKeys["sk-virt-stored"] = true
	srv := f.server(t)

	client := NewAdminClient(srv.URL, "admin", "secret")
	got, err := client.EnsureOrganization(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		PlanID:         "plan-pro",
		ExistingKey:    "sk-virt-stored",
	})
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}

	if got.VirtualKey != "sk-virt-stored" {
		t.Errorf("key = %q, want the stored one reused", got.VirtualKey)
	}
	if got.KeyIssued {
		t.Error("KeyIssued = true; a reused key must not be reported as new, or every page render writes to the organisation row")
	}
	// The one that actually matters: minting here would have revoked the very
	// key this call was handed.
	if n := f.keysIssued.Load(); n != 0 {
		t.Errorf("minted %d key(s) while a working one was stored; that revokes the working one", n)
	}
	if plan := f.lastPlanID.Load(); plan != "plan-pro" {
		t.Errorf("plan sent = %v, want plan-pro; the account must be brought to the current plan even when the key is reused", plan)
	}
}

func TestEnsureOrganizationReplacesARevokedKey(t *testing.T) {
	f := newFakeGateway()
	srv := f.server(t) // acceptedKeys is empty: the stored key is rejected

	client := NewAdminClient(srv.URL, "admin", "secret")
	got, err := client.EnsureOrganization(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		ExistingKey:    "sk-virt-revoked",
	})
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if got.VirtualKey == "sk-virt-revoked" || got.VirtualKey == "" {
		t.Errorf("key = %q, want a freshly minted replacement", got.VirtualKey)
	}
	if !got.KeyIssued {
		t.Error("KeyIssued = false; the caller must be told to persist the replacement")
	}
}

func TestEnsureOrganizationPrefersAnExistingGatewayKeyOverMinting(t *testing.T) {
	f := newFakeGateway()
	f.existingKeys = []GatewayVirtualKey{
		{ID: "old", UserID: "org-7", Status: "revoked", Key: "sk-virt-dead"},
		{ID: "cur", UserID: "org-7", Status: "active", Key: "sk-virt-live"},
	}
	f.acceptedKeys["sk-virt-live"] = true
	srv := f.server(t)

	client := NewAdminClient(srv.URL, "admin", "secret")
	got, err := client.EnsureOrganization(context.Background(), OrganizationSpec{OrganizationID: 7})
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if got.VirtualKey != "sk-virt-live" {
		t.Errorf("key = %q, want the active key the Gateway already held", got.VirtualKey)
	}
	if n := f.keysIssued.Load(); n != 0 {
		t.Errorf("minted %d key(s) despite an active one existing", n)
	}
}

func TestEnsureOrganizationKeepsTheStoredKeyWhenTheGatewayCannotBeAsked(t *testing.T) {
	// A Gateway that answers /api/users but whose key check is unreachable
	// stands in for a transient outage. Treating "cannot check" as "revoked"
	// would rotate every tenant's credentials during a five-second blip.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close() // no response at all
		}
	})
	minted := false
	mux.HandleFunc("/api/keys", func(w http.ResponseWriter, _ *http.Request) {
		minted = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(GatewayVirtualKey{Key: "sk-virt-new"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewAdminClient(srv.URL, "admin", "secret")
	got, err := client.EnsureOrganization(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		ExistingKey:    "sk-virt-stored",
	})
	if err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if got.VirtualKey != "sk-virt-stored" {
		t.Errorf("key = %q, want the stored key kept while the Gateway is unreachable", got.VirtualKey)
	}
	if minted {
		t.Error("minted a key because the check could not be performed; an outage must not rotate credentials")
	}
}

func TestEnsureOrganizationDefaultsThePlanRatherThanSendingNone(t *testing.T) {
	f := newFakeGateway()
	f.acceptedKeys["sk-virt-stored"] = true
	srv := f.server(t)

	client := NewAdminClient(srv.URL, "admin", "secret")
	if _, err := client.EnsureOrganization(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		ExistingKey:    "sk-virt-stored",
	}); err != nil {
		t.Fatalf("EnsureOrganization: %v", err)
	}
	if plan := f.lastPlanID.Load(); plan != FallbackPlanID {
		t.Errorf("plan sent = %v, want %q", plan, FallbackPlanID)
	}
}

func TestSyncOrganizationPlanReportsRejection(t *testing.T) {
	// The predecessor performed the PUT and returned nil whatever came back, so
	// a rejected plan change was indistinguishable from a successful one.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unknown plan"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewAdminClient(srv.URL, "admin", "secret")
	err := client.SyncOrganizationPlan(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		PlanID:         "plan-that-does-not-exist",
	})
	if err == nil {
		t.Fatal("SyncOrganizationPlan returned nil for a rejected update")
	}
}

func TestSyncOrganizationPlanFallsBackToUpdateWhenTheUserExists(t *testing.T) {
	f := newFakeGateway()
	f.userPostStatus = http.StatusConflict
	srv := f.server(t)

	client := NewAdminClient(srv.URL, "admin", "secret")
	if err := client.SyncOrganizationPlan(context.Background(), OrganizationSpec{
		OrganizationID: 7,
		PlanID:         "plan-enterprise",
	}); err != nil {
		t.Fatalf("SyncOrganizationPlan: %v", err)
	}
	if f.usersPut.Load() != 1 {
		t.Errorf("PUT /api/users called %d times, want 1", f.usersPut.Load())
	}
	if plan := f.lastPlanID.Load(); plan != "plan-enterprise" {
		t.Errorf("plan sent = %v, want plan-enterprise", plan)
	}
}
