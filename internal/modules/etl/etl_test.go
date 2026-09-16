package etl_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/etl"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestLegacyDatetimeTransformer(t *testing.T) {
	// 1. Valid MySQL datetime
	raw := "2024-03-15 10:30:00"
	parsed, err := etl.ParseLegacyDatetime(raw)
	if err != nil {
		t.Fatalf("ParseLegacyDatetime failed: %v", err)
	}
	if parsed == nil || parsed.UTC().Year() != 2024 || parsed.UTC().Month() != time.March {
		t.Errorf("unexpected parsed time: %v", parsed)
	}

	// 2. MySQL zero datetime
	zeroRaw := "0000-00-00 00:00:00"
	zeroParsed, err := etl.ParseLegacyDatetime(zeroRaw)
	if err != nil {
		t.Fatalf("ParseLegacyDatetime for zero failed: %v", err)
	}
	if zeroParsed != nil {
		t.Errorf("expected nil for zero datetime, got %v", zeroParsed)
	}
}

func TestLegacyMoneyTransformer(t *testing.T) {
	// String money
	m1, err := etl.ParseLegacyMoney("145.75")
	if err != nil || m1 != money.MustParse("145.75") {
		t.Errorf("ParseLegacyMoney string failed: %v, got %v", err, m1)
	}

	// Float money
	m2, err := etl.ParseLegacyMoney(250.50)
	if err != nil || m2 != money.MustParse("250.50") {
		t.Errorf("ParseLegacyMoney float failed: %v, got %v", err, m2)
	}
}

func TestLegacyStatusMapping(t *testing.T) {
	cases := map[string]string{
		"0":         "pending",
		"1":         "confirmed",
		"3":         "shipped",
		"4":         "delivered",
		"accept":    "confirmed",
		"cancel":    "cancelled",
		"delivered": "delivered",
	}

	for input, expected := range cases {
		mapped := etl.MapLegacyOrderStatus(input)
		if mapped != expected {
			t.Errorf("MapLegacyOrderStatus(%q) = %q; want %q", input, mapped, expected)
		}
	}
}

func TestETLVerificationGates(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pipeline := etl.NewPipeline(logger)

	// Gate 1: Exact row match and money sum match
	res1 := pipeline.RunVerificationGate(
		ctx,
		"orders",
		1500,
		1500,
		money.MustParse("345000.75"),
		money.MustParse("345000.75"),
	)
	if !res1.ChecksumMatches || len(res1.Errors) > 0 {
		t.Errorf("expected gate 1 to pass, got: %+v", res1)
	}

	// Gate 2: Row count mismatch
	res2 := pipeline.RunVerificationGate(
		ctx,
		"users",
		4417,
		4416, // Missing 1 row
		money.Zero,
		money.Zero,
	)
	if res2.ChecksumMatches || len(res2.Errors) == 0 {
		t.Errorf("expected gate 2 to fail on missing row")
	}

	// Gate 3: Money discrepancy
	res3 := pipeline.RunVerificationGate(
		ctx,
		"wallet_transactions",
		200,
		200,
		money.MustParse("1000.00"),
		money.MustParse("999.99"), // 1 cent discrepancy
	)
	if res3.ChecksumMatches || len(res3.Errors) == 0 {
		t.Errorf("expected gate 3 to fail on 1 cent discrepancy")
	}

	report := pipeline.CompileMigrationReport(time.Now().Add(-time.Second), []*etl.ValidationResult{res1, res2, res3})
	if report.AllGatesPassed {
		t.Errorf("expected report.AllGatesPassed to be false")
	}
}
