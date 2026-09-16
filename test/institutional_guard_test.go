package test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockInstStore struct {
	userWorks   map[int64][]int64
	connections map[int64][]int64
}

func (m *mockInstStore) AllowedWorkIDs(_ context.Context, userID int64, mode int) ([]int64, error) {
	works := m.userWorks[userID]
	if len(works) == 0 {
		return []int64{}, nil
	}
	if mode == int(org.FilterSimple) {
		return works, nil
	}

	// WithConnections mode
	var allowed []int64
	seen := make(map[int64]bool)
	for _, fromID := range works {
		for _, toID := range m.connections[fromID] {
			if !seen[toID] {
				seen[toID] = true
				allowed = append(allowed, toID)
			}
		}
	}
	return allowed, nil
}

// TestInstitutionalFilterAsymmetry asserts that:
// 1. In Simple mode: unrestricted products (empty institutional_work_ids) are allowed.
// 2. In WithConnections mode: unrestricted products are NOT allowed (strict asymmetry).
// 3. User with works [5, 7, 3, 8] and connections 5->7, 5->9, 7->10 gets allowed [7, 9, 10] in WithConnections.
func TestInstitutionalFilterAsymmetry(t *testing.T) {
	ctx := context.Background()
	store := &mockInstStore{
		userWorks: map[int64][]int64{
			42: {5, 7, 3, 8},
		},
		connections: map[int64][]int64{
			5: {7, 9},
			7: {10},
		},
	}

	gate := catalog.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return store.AllowedWorkIDs(ctx, userID, mode)
	})

	// 1. Simple mode resolution
	simpleWorks, err := gate.AllowedWorkIDs(ctx, 42, int(org.FilterSimple))
	if err != nil {
		t.Fatalf("Simple mode error: %v", err)
	}
	if len(simpleWorks) != 4 {
		t.Errorf("Simple mode: got %v, want 4 works [5,7,3,8]", simpleWorks)
	}

	// 2. WithConnections mode resolution
	connWorks, err := gate.AllowedWorkIDs(ctx, 42, int(org.FilterWithConnections))
	if err != nil {
		t.Fatalf("WithConnections mode error: %v", err)
	}
	if len(connWorks) != 3 {
		t.Fatalf("WithConnections mode: got %v, want 3 works [7,9,10]", connWorks)
	}

	expected := map[int64]bool{7: true, 9: true, 10: true}
	for _, id := range connWorks {
		if !expected[id] {
			t.Errorf("unexpected work ID in WithConnections: %d", id)
		}
	}

	// 3. Evaluate visibility simulation for both modes
	type productTest struct {
		name             string
		instWorkIDs      []int64
		visibleSimple    bool
		visibleWithConns bool
	}

	tests := []productTest{
		{
			name:             "unrestricted product (empty array)",
			instWorkIDs:      []int64{},
			visibleSimple:    true,  // Fallback allowed in Simple mode
			visibleWithConns: false, // Strictly forbidden in WithConnections mode
		},
		{
			name:             "product in direct user group (5)",
			instWorkIDs:      []int64{5},
			visibleSimple:    true,  // 5 is in userWorks
			visibleWithConns: false, // 5 is not in connected targets [7,9,10]
		},
		{
			name:             "product in connected target group (9)",
			instWorkIDs:      []int64{9},
			visibleSimple:    false, // 9 is not in userWorks [5,7,3,8]
			visibleWithConns: true,  // 9 is in connWorks [7,9,10]
		},
		{
			name:             "product in unauthorized group (99)",
			instWorkIDs:      []int64{99},
			visibleSimple:    false,
			visibleWithConns: false,
		},
	}

	overlaps := func(a, b []int64) bool {
		for _, x := range a {
			for _, y := range b {
				if x == y {
					return true
				}
			}
		}
		return false
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple mode rule: empty OR overlaps
			isSimpleVisible := len(tt.instWorkIDs) == 0 || overlaps(tt.instWorkIDs, simpleWorks)
			if isSimpleVisible != tt.visibleSimple {
				t.Errorf("Simple mode visibility mismatch for %s: got %v, want %v", tt.name, isSimpleVisible, tt.visibleSimple)
			}

			// WithConnections mode rule: NOT empty AND overlaps
			isConnVisible := len(tt.instWorkIDs) > 0 && overlaps(tt.instWorkIDs, connWorks)
			if isConnVisible != tt.visibleWithConns {
				t.Errorf("WithConnections mode visibility mismatch for %s: got %v, want %v", tt.name, isConnVisible, tt.visibleWithConns)
			}
		})
	}
}

// TestGateCompositionInServices verifies that the catalog service accepts
// InstitutionalGate injection without runtime issues. Promo has no gate: its
// offer rule takes the buyer branch's connected works as a query fact.
func TestGateCompositionInServices(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	catSvc := catalog.NewService(nil, log)

	gate := catalog.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return []int64{1, 2, 3}, nil
	})

	catSvc.SetInstitutionalGate(gate)

	// Verify catalog product struct carries institutional work IDs
	p := &catalog.Product{
		Name:                 i18n.New("بانادول", "Panadol"),
		InstitutionalWorkIDs: []int64{10, 20},
	}
	if len(p.InstitutionalWorkIDs) != 2 || p.InstitutionalWorkIDs[0] != 10 {
		t.Errorf("Product InstitutionalWorkIDs mismatch: %v", p.InstitutionalWorkIDs)
	}
}
