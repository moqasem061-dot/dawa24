package test

import (
	"encoding/json"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TestMoneyRoundTripPerModule tests money serialization, arithmetic, JSON roundtrip,
// and precision to the exact cent across positive, negative, and zero amounts.
func TestMoneyRoundTripPerModule(t *testing.T) {
	testCases := []struct {
		name      string
		minor     int64
		str       string
		isNeg     bool
		isZero    bool
		formatted string
	}{
		{
			name:      "zero amount",
			minor:     0,
			str:       "0.00",
			isNeg:     false,
			isZero:    true,
			formatted: "0.00 EGP",
		},
		{
			name:      "positive small amount",
			minor:     150, // 1.50 EGP
			str:       "1.50",
			isNeg:     false,
			isZero:    false,
			formatted: "1.50 EGP",
		},
		{
			name:      "positive large amount",
			minor:     50000000, // 500,000.00 EGP
			str:       "500000.00",
			isNeg:     false,
			isZero:    false,
			formatted: "500,000.00 EGP",
		},
		{
			name:      "negative adjustment amount",
			minor:     -2550, // -25.50 EGP
			str:       "-25.50",
			isNeg:     true,
			isZero:    false,
			formatted: "-25.50 EGP",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := money.FromMinor(tc.minor)
			if m.Minor() != tc.minor {
				t.Errorf("got minor %d, want %d", m.Minor(), tc.minor)
			}
			if m.IsZero() != tc.isZero {
				t.Errorf("got IsZero=%v, want %v", m.IsZero(), tc.isZero)
			}
			if m.IsNegative() != tc.isNeg {
				t.Errorf("got IsNegative=%v, want %v", m.IsNegative(), tc.isNeg)
			}

			// Parse test
			parsed, err := money.Parse(tc.str)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tc.str, err)
			}
			if parsed.Minor() != tc.minor {
				t.Errorf("Parse(%q) = %d, want %d", tc.str, parsed.Minor(), tc.minor)
			}

			// JSON round-trip
			type DTO struct {
				Price money.Amount `json:"price"`
			}
			data, err := json.Marshal(DTO{Price: m})
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			var unmarshaled DTO
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if unmarshaled.Price.Minor() != tc.minor {
				t.Errorf("JSON roundtrip got %d, want %d", unmarshaled.Price.Minor(), tc.minor)
			}

			// Arithmetic test: Addition & Subtraction
			added, err := m.Add(money.FromMinor(100))
			if err != nil {
				t.Fatalf("Add failed: %v", err)
			}
			if added.Minor() != tc.minor+100 {
				t.Errorf("Add result = %d, want %d", added.Minor(), tc.minor+100)
			}

			subbed, err := added.Sub(money.FromMinor(100))
			if err != nil {
				t.Fatalf("Sub failed: %v", err)
			}
			if subbed.Minor() != tc.minor {
				t.Errorf("Sub result = %d, want %d", subbed.Minor(), tc.minor)
			}
		})
	}
}
