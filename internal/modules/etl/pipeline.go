package etl

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Pipeline coordinates the 6-stage data ETL execution from legacy storage to modern PostgreSQL.
type Pipeline struct {
	log *slog.Logger
}

// NewPipeline creates a migration engine.
func NewPipeline(log *slog.Logger) *Pipeline {
	return &Pipeline{log: log}
}

// RunVerificationGate executes checksum and monetary sum reconciliation for a migrated table.
func (p *Pipeline) RunVerificationGate(
	ctx context.Context,
	tableName string,
	sourceRows, targetRows int64,
	sourceMoneySum, targetMoneySum money.Amount,
) *ValidationResult {
	res := &ValidationResult{
		Table:           tableName,
		SourceRows:      sourceRows,
		MigratedRows:    targetRows,
		SourceMoneySum:  sourceMoneySum,
		TargetMoneySum:  targetMoneySum,
		ChecksumMatches: true,
		OrphanFKCount:   0,
	}

	if sourceRows != targetRows {
		res.ChecksumMatches = false
		res.Errors = append(res.Errors, fmt.Sprintf("Row count mismatch: source=%d, target=%d", sourceRows, targetRows))
	}

	if sourceMoneySum != targetMoneySum {
		res.ChecksumMatches = false
		res.Errors = append(res.Errors, fmt.Sprintf("Monetary sum mismatch to the cent: source=%s, target=%s", sourceMoneySum, targetMoneySum))
	}

	if len(res.Errors) > 0 {
		p.log.WarnContext(ctx, "ETL verification gate failed", "table", tableName, "errors", res.Errors)
	} else {
		p.log.InfoContext(ctx, "ETL verification gate passed", "table", tableName, "rows", targetRows)
	}

	return res
}

// CompileMigrationReport summarizes the results across all verified tables.
func (p *Pipeline) CompileMigrationReport(startedAt time.Time, results []*ValidationResult) *MigrationReport {
	completedAt := time.Now().UTC()
	report := &MigrationReport{
		TotalTables:    len(results),
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		Duration:       completedAt.Sub(startedAt),
		Results:        results,
		AllGatesPassed: true,
	}

	for _, r := range results {
		report.TotalRows += r.MigratedRows
		if !r.ChecksumMatches || len(r.Errors) > 0 {
			report.AllGatesPassed = false
		}
	}

	return report
}
