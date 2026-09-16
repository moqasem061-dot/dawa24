// Package etl implements the six-stage migration pipeline for transitioning legacy
// MariaDB records to modern partitioned PostgreSQL schema while preserving PKs.
package etl

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Stage defines the current ETL execution stage.
type Stage string

const (
	StageExtract   Stage = "extract"
	StageValidate  Stage = "validate"
	StageTransform Stage = "transform"
	StageLoad      Stage = "load"
	StageVerify    Stage = "verify"
	StageReconcile Stage = "reconcile"
)

// TableMigrationConfig specifies transformation and mapping rules for a legacy table.
type TableMigrationConfig struct {
	SourceTable string
	TargetTable string
	PrimaryKeys []string
	BatchSize   int
}

// ValidationResult records integrity checks across migrated tables.
type ValidationResult struct {
	Table           string       `json:"table"`
	SourceRows      int64        `json:"source_rows"`
	MigratedRows    int64        `json:"migrated_rows"`
	SourceMoneySum  money.Amount `json:"source_money_sum"`
	TargetMoneySum  money.Amount `json:"target_money_sum"`
	ChecksumMatches bool         `json:"checksum_matches"`
	OrphanFKCount   int64        `json:"orphan_fk_count"`
	Errors          []string     `json:"errors,omitempty"`
}

// MigrationReport aggregates verification metrics after an ETL migration run.
type MigrationReport struct {
	TotalTables    int                 `json:"total_tables"`
	TotalRows      int64               `json:"total_rows"`
	StartedAt      time.Time           `json:"started_at"`
	CompletedAt    time.Time           `json:"completed_at"`
	Duration       time.Duration       `json:"duration"`
	Results        []*ValidationResult `json:"results"`
	AllGatesPassed bool                `json:"all_gates_passed"`
}

// LegacyOrderStatusMap defines explicit mappings from legacy status strings to canonical OrderStatus.
var LegacyOrderStatusMap = map[string]string{
	"0":         "pending",
	"1":         "confirmed",
	"2":         "processing",
	"3":         "shipped",
	"4":         "delivered",
	"5":         "cancelled",
	"pending":   "pending",
	"accept":    "confirmed",
	"complete":  "delivered",
	"cancel":    "cancelled",
	"delivered": "delivered",
}

// MapLegacyOrderStatus maps legacy status codes or strings to the target enum.
func MapLegacyOrderStatus(raw string) string {
	if mapped, ok := LegacyOrderStatusMap[raw]; ok {
		return mapped
	}
	return "pending"
}
