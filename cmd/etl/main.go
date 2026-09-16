package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	sourceDSN := flag.String("source-dsn", "", "MariaDB source DSN (e.g. user:pass@tcp(localhost:3306)/dawa24)")
	targetDSN := flag.String("target-dsn", "", "PostgreSQL target DSN (e.g. postgres://user:pass@localhost:5432/dawa24)")
	verifyOnly := flag.Bool("verify-only", false, "Run verification gates without loading data")
	batchSize := flag.Int("batch-size", 1000, "ETL streaming batch size")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *sourceDSN == "" || *targetDSN == "" {
		logger.Error("Both --source-dsn and --target-dsn are required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	logger.Info("starting Dawa24 MariaDB to PostgreSQL migration pipeline",
		"verify_only", *verifyOnly,
		"batch_size", *batchSize,
	)

	pipeline := NewPipeline(*sourceDSN, *targetDSN, *batchSize, logger)
	if err := pipeline.Run(ctx, *verifyOnly); err != nil {
		logger.Error("ETL migration pipeline failed", "error", err)
		os.Exit(1)
	}

	logger.Info("ETL migration pipeline finished successfully")
}
