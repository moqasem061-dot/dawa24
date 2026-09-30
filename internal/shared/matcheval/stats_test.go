package matcheval

import (
	"math"
	"testing"
)

func TestClopperPearsonLower(t *testing.T) {
	tests := []struct {
		name       string
		successes  int
		n          int
		confidence float64
		want       float64
		tol        float64
	}{
		{
			name:       "995 of 1000 at 0.95",
			successes:  995,
			n:          1000,
			confidence: 0.95,
			want:       0.98942,
			tol:        1e-4,
		},
		{
			name:       "0 of 10",
			successes:  0,
			n:          10,
			confidence: 0.95,
			want:       0.0,
			tol:        1e-6,
		},
		{
			name:       "10 of 10 at 0.95",
			successes:  10,
			n:          10,
			confidence: 0.95,
			want:       0.7411,
			tol:        1e-4,
		},
		{
			name:       "n=0",
			successes:  0,
			n:          0,
			confidence: 0.95,
			want:       0.0,
			tol:        1e-6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClopperPearsonLower(tt.successes, tt.n, tt.confidence)
			if math.Abs(got-tt.want) > tt.tol {
				t.Errorf("ClopperPearsonLower(%d, %d, %v) = %v, want %v (±%v)",
					tt.successes, tt.n, tt.confidence, got, tt.want, tt.tol)
			}
		})
	}
}

func TestWilson(t *testing.T) {
	tests := []struct {
		name      string
		successes int
		n         int
		z         float64
		wantLo    float64
		wantHi    float64
		tol       float64
	}{
		{
			name:      "n=0",
			successes: 0,
			n:         0,
			z:         1.96,
			wantLo:    0,
			wantHi:    0,
			tol:       1e-6,
		},
		{
			name:      "50 of 100 at z=1.96",
			successes: 50,
			n:         100,
			z:         1.96,
			wantLo:    0.4038,
			wantHi:    0.5962,
			tol:       1e-3,
		},
		{
			name:      "0 of 10 at z=1.96",
			successes: 0,
			n:         10,
			z:         1.96,
			wantLo:    0.0,
			wantHi:    0.2775,
			tol:       1e-3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLo, gotHi := Wilson(tt.successes, tt.n, tt.z)
			if math.Abs(gotLo-tt.wantLo) > tt.tol || math.Abs(gotHi-tt.wantHi) > tt.tol {
				t.Errorf("Wilson(%d, %d, %v) = (%v, %v), want (%v, %v) (±%v)",
					tt.successes, tt.n, tt.z, gotLo, gotHi, tt.wantLo, tt.wantHi, tt.tol)
			}
		})
	}
}
