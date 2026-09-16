package actions

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// memStore is an in-memory Store with the same single-use claim semantics as
// the Postgres one.
type memStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*Pending
	next int64
}

func newMemStore() *memStore { return &memStore{rows: map[uuid.UUID]*Pending{}} }

func (m *memStore) CreatePendingAction(_ context.Context, p *Pending) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	p.ID, p.PublicID, p.CreatedAt = m.next, uuid.New(), time.Now()
	cp := *p
	m.rows[p.PublicID] = &cp
	return nil
}

func (m *memStore) GetPendingAction(_ context.Context, id uuid.UUID, userID, orgID int64) (*Pending, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.rows[id]
	if !ok || p.UserID != userID || p.OrganizationID != orgID {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (m *memStore) byID(id int64) *Pending {
	for _, p := range m.rows {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (m *memStore) ClaimPendingAction(_ context.Context, id int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.byID(id)
	if p == nil || p.Status != StatusPending || !time.Now().Before(p.ExpiresAt) {
		return false, nil
	}
	p.Status = StatusExecuting
	return true, nil
}

func (m *memStore) FinishPendingAction(_ context.Context, id int64, s Status, o *Outcome, errText string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.byID(id); p != nil && p.Status == StatusExecuting {
		p.Status, p.Outcome, p.Error = s, o, errText
	}
	return nil
}

func (m *memStore) DecidePendingAction(_ context.Context, id int64, s Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.byID(id); p != nil && p.Status == StatusPending {
		p.Status = s
	}
	return nil
}

// fakeExec offers one action whose preview reflects mutable "live data".
type fakeExec struct {
	mu        sync.Mutex
	price     string
	permitted bool
	refuse    string
	executed  int
}

var addDef = Definition{
	Name: "cart_add", Label: "إضافة", Risk: RiskLow, Description: "add",
	Params: []Param{
		{Name: "offer", Type: ParamRef, RefKind: handles.KindVariant, Required: true},
		{Name: "quantity", Type: ParamInt, Required: true, Min: 1, Max: 100},
	},
}

func (f *fakeExec) Definitions(authctx.Actor) []Definition { return []Definition{addDef} }
func (f *fakeExec) Permitted(authctx.Actor, string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.permitted
}
func (f *fakeExec) Prepare(_ context.Context, _ authctx.Actor, _ string, args Args) (Preview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse != "" {
		return Preview{}, Refuse("%s", f.refuse)
	}
	return Preview{Title: "إضافة", Summary: f.price, Details: []Detail{{Label: "offer", Value: "x"}}}, nil
}
func (f *fakeExec) Execute(context.Context, authctx.Actor, string, Args) (Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed++
	return Outcome{Message: "done"}, nil
}

func pharmacist(orgID, userID int64) authctx.Actor {
	a := authctx.Actor{UserID: userID, OrgID: orgID, OrganizationID: orgID, Scope: rbac.ScopePharmacy}
	a.Grants([]string{"pharmacy.assistant.use", "pharmacy.assistant.act", "pharmacy.cart.use"})
	return a
}

type harness struct {
	flow  *Flow
	store *memStore
	exec  *fakeExec
	gate  bool
}

func newHarness() *harness {
	h := &harness{store: newMemStore(), exec: &fakeExec{price: "10.00", permitted: true}, gate: true}
	h.flow = NewFlow(h.store, h.exec, func(authctx.Actor) bool { return h.gate }, nil)
	return h
}

func resolveOK(kind handles.Kind, token string) (int64, error) {
	if kind == handles.KindVariant && token == "good" {
		return 7, nil
	}
	return 0, errors.New("bad")
}

func (h *harness) propose(t *testing.T, a authctx.Actor) *Pending {
	t.Helper()
	p, err := h.flow.Propose(context.Background(), a, ProposeRequest{
		Action: "cart_add", Args: json.RawMessage(`{"offer":"good","quantity":3}`), Resolve: resolveOK,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return p
}

func TestProposeStoresVerifiedArgsAndChangesNothing(t *testing.T) {
	h := newHarness()
	p := h.propose(t, pharmacist(1, 10))
	if p.Status != StatusPending || p.Args.ID("offer") != 7 || p.Args.Int("quantity") != 3 {
		t.Fatalf("unexpected proposal: %+v", p)
	}
	if h.exec.executed != 0 {
		t.Fatal("proposing must not execute")
	}
}

func TestProposeRefusesBadArguments(t *testing.T) {
	h := newHarness()
	a := pharmacist(1, 10)
	for _, raw := range []string{
		`{"offer":"good"}`,
		`{"offer":"good","quantity":0}`,
		`{"offer":"good","quantity":3,"org_id":2}`,
		`{"offer":"7","quantity":3}`,
		`[1,2]`,
	} {
		_, err := h.flow.Propose(context.Background(), a, ProposeRequest{Action: "cart_add", Args: json.RawMessage(raw), Resolve: resolveOK})
		var invalid *ErrInvalid
		if err == nil || (!errors.As(err, &invalid) && !errors.Is(err, ErrBadRef)) {
			t.Errorf("%s: want an argument refusal, got %v", raw, err)
		}
	}
	if _, err := h.flow.Propose(context.Background(), a, ProposeRequest{Action: "order_everything", Resolve: resolveOK}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("unknown action: got %v", err)
	}
}

func TestConfirmExecutesOnceForItsOwner(t *testing.T) {
	h := newHarness()
	owner := pharmacist(1, 10)
	p := h.propose(t, owner)

	if _, err := h.flow.Confirm(context.Background(), pharmacist(1, 11), p.PublicID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user must not confirm: %v", err)
	}
	if _, err := h.flow.Confirm(context.Background(), pharmacist(2, 10), p.PublicID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the same user in another organisation must not confirm: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.flow.Confirm(context.Background(), owner, p.PublicID)
		}()
	}
	wg.Wait()
	if h.exec.executed != 1 {
		t.Fatalf("executed %d times, want exactly once", h.exec.executed)
	}
	got, _ := h.flow.Get(context.Background(), owner, p.PublicID)
	if got.Status != StatusExecuted || got.Outcome == nil {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestConfirmRefusesWhenTheDataChanged(t *testing.T) {
	h := newHarness()
	a := pharmacist(1, 10)
	p := h.propose(t, a)
	h.exec.price = "12.00"

	_, err := h.flow.Confirm(context.Background(), a, p.PublicID)
	var stale *ErrStale
	if !errors.As(err, &stale) || h.exec.executed != 0 {
		t.Fatalf("a changed preview must be refused, got %v (executed %d)", err, h.exec.executed)
	}
}

func TestConfirmRechecksAuthorityAtConfirmTime(t *testing.T) {
	h := newHarness()
	a := pharmacist(1, 10)

	p := h.propose(t, a)
	h.exec.permitted = false
	if _, err := h.flow.Confirm(context.Background(), a, p.PublicID); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("revoked permission: %v", err)
	}

	h.exec.permitted, h.gate = true, false
	if _, err := h.flow.Confirm(context.Background(), a, p.PublicID); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("revoked act grant: %v", err)
	}

	h.gate = true
	vendor := a
	vendor.Scope = rbac.ScopeVendor
	if _, err := h.flow.Confirm(context.Background(), vendor, p.PublicID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a different dashboard must not confirm: %v", err)
	}

	h.exec.refuse = "out of stock"
	_, err := h.flow.Confirm(context.Background(), a, p.PublicID)
	if r, ok := AsRefusal(err); !ok || r.Message != "out of stock" {
		t.Fatalf("a refusal at confirm time must surface, got %v", err)
	}
	if h.exec.executed != 0 {
		t.Fatal("nothing should have executed")
	}
}

func TestExpiredAndCancelledProposalsCannotRun(t *testing.T) {
	h := newHarness()
	a := pharmacist(1, 10)

	p := h.propose(t, a)
	h.flow.now = func() time.Time { return time.Now().Add(TTL + time.Minute) }
	if _, err := h.flow.Confirm(context.Background(), a, p.PublicID); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}

	h.flow.now = time.Now
	q := h.propose(t, a)
	if _, err := h.flow.Cancel(context.Background(), a, q.PublicID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.flow.Confirm(context.Background(), a, q.PublicID); !errors.Is(err, ErrDecided) {
		t.Fatalf("cancelled: %v", err)
	}
	if h.exec.executed != 0 {
		t.Fatal("nothing should have executed")
	}
}

func TestSignatureDescribesArguments(t *testing.T) {
	if got := Signature(addDef); got != "cart_add(offer: ref, quantity: int 1-100)" {
		t.Fatalf("signature = %q", got)
	}
}
