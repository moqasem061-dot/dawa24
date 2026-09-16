package workflow

import (
	"testing"
)

func TestWeeklyCoverageValidation(t *testing.T) {
	validLat := 30.0444
	validLng := 31.2357
	invalidLatHigh := 95.0
	invalidLatLow := -95.0
	invalidLngHigh := 185.0
	invalidLngLow := -185.0

	tests := []struct {
		name    string
		cov     WeeklyCoverage
		wantErr bool
	}{
		{
			name: "valid coverage Sunday",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      0,
				DistanceMeters: 25000,
				Latitude:       &validLat,
				Longitude:      &validLng,
			},
			wantErr: false,
		},
		{
			name: "valid coverage Saturday boundary",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      6,
				DistanceMeters: 500000,
				Latitude:       &validLat,
				Longitude:      &validLng,
			},
			wantErr: false,
		},
		{
			name: "invalid branch ID zero",
			cov: WeeklyCoverage{
				BranchID:       0,
				DayOfWeek:      1,
				DistanceMeters: 1000,
			},
			wantErr: true,
		},
		{
			name: "invalid day negative",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      -1,
				DistanceMeters: 1000,
			},
			wantErr: true,
		},
		{
			name: "invalid day 7",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      7,
				DistanceMeters: 1000,
			},
			wantErr: true,
		},
		{
			name: "invalid zero distance",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      2,
				DistanceMeters: 0,
			},
			wantErr: true,
		},
		{
			name: "invalid distance exceeding max 500km",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      2,
				DistanceMeters: 500001,
			},
			wantErr: true,
		},
		{
			name: "invalid latitude high",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      3,
				DistanceMeters: 1000,
				Latitude:       &invalidLatHigh,
				Longitude:      &validLng,
			},
			wantErr: true,
		},
		{
			name: "invalid latitude low",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      3,
				DistanceMeters: 1000,
				Latitude:       &invalidLatLow,
				Longitude:      &validLng,
			},
			wantErr: true,
		},
		{
			name: "invalid longitude high",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      4,
				DistanceMeters: 1000,
				Latitude:       &validLat,
				Longitude:      &invalidLngHigh,
			},
			wantErr: true,
		},
		{
			name: "invalid longitude low",
			cov: WeeklyCoverage{
				BranchID:       1,
				DayOfWeek:      4,
				DistanceMeters: 1000,
				Latitude:       &validLat,
				Longitude:      &invalidLngLow,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cov.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
