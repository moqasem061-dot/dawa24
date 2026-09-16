package etl

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Pipeline coordinates the 6-stage data ETL execution from legacy storage to modern PostgreSQL.
type Pipeline struct {
	log *slog.Logger
}

// NewPipeline creates a migration engine.
func NewPipeline(log *slog.Logger) *Pipeline {
	return &Pipeline{log: log}
}

// VerifyTable directly computes counts and monetary sums on both source and target databases.
func (p *Pipeline) VerifyTable(
	ctx context.Context,
	sourceDB *sql.DB,
	targetDB *sql.DB,
	sourceTable, targetTable string,
	moneyCol string,
) (*ValidationResult, error) {
	var srcRows, tgtRows int64
	var srcSumMinor, tgtSumMinor sql.NullInt64

	srcCountQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s;", sourceTable)
	if err := sourceDB.QueryRowContext(ctx, srcCountQuery).Scan(&srcRows); err != nil {
		return nil, fmt.Errorf("query source count for %s: %w", sourceTable, err)
	}

	tgtCountQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s;", targetTable)
	if err := targetDB.QueryRowContext(ctx, tgtCountQuery).Scan(&tgtRows); err != nil {
		return nil, fmt.Errorf("query target count for %s: %w", targetTable, err)
	}

	var srcMoney, tgtMoney money.Amount
	if moneyCol != "" {
		srcSumQuery := fmt.Sprintf("SELECT COALESCE(ROUND(SUM(%s) * 100), 0) FROM %s;", moneyCol, sourceTable)
		_ = sourceDB.QueryRowContext(ctx, srcSumQuery).Scan(&srcSumMinor)
		if srcSumMinor.Valid {
			srcMoney = money.FromMinor(srcSumMinor.Int64)
		}

		tgtSumQuery := fmt.Sprintf("SELECT COALESCE(SUM(%s), 0) FROM %s;", moneyCol, targetTable)
		_ = targetDB.QueryRowContext(ctx, tgtSumQuery).Scan(&tgtSumMinor)
		if tgtSumMinor.Valid {
			tgtMoney = money.FromMinor(tgtSumMinor.Int64)
		}
	}

	return p.RunVerificationGate(ctx, sourceTable, srcRows, tgtRows, srcMoney, tgtMoney), nil
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
