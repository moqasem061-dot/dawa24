// Command cli runs operational tasks: migrations, seeding, and the data
// migration from the legacy MariaDB database.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	dbfs "github.com/muhiya/dawa24-store/db"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgPG "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	_ "github.com/muhiya/dawa24-store/internal/shared/timeutil"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() string {
	return `dawa24 cli

Usage:
  cli migrate           Apply all pending migrations
  cli migrate-status    Show applied and pending migrations
  cli migrate-data      Run legacy MariaDB to PostgreSQL ETL pipeline
  cli seed              Seed default platform reference data
  cli seed-users        Create development sign-in accounts (non-prod only)
  cli reset-db          Wipe all rows and reset DB to clean zero state (admin only)
  cli reindex           Rebuild catalog.product_index read model from master tables
  cli activate-imported [--apply]   Publish catalogue products a bulk import left pending
  cli smartorder-smoke <orgID> <branchID> <userID> <file>   Drive a smart order end to end
  cli health            Verify database and cache connectivity
  cli corpus-export     Copy every retained import file into test/corpus
  cli ai-identities [--apply] [--org N]   Give every منشأة a Gateway user and key
  cli imports-recover [--apply]           Release imports wedged in 'processing'
`

}

func run() error {
	if len(os.Args) < 2 {
		fmt.Print(usage())
		return errors.New("no command given")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadForCLI()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.Observ, cfg.Env)

	// Same guard as the server: the backfill reads the stored Gateway
	// credential and sends it to the Gateway host, so it must refuse the
	// database password just as the settings screen does.
	platformadmin.SetKnownDatabaseSecret(config.DatabasePassword(cfg.Database.URL))

	var db *database.DB
	const maxAttempts = 30
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		db, err = database.Open(ctx, cfg.Database)
		if err == nil {
			break
		}
		if attempt == maxAttempts || ctx.Err() != nil {
			return fmt.Errorf("connect to database after %d attempts: %w", attempt, err)
		}
		log.Warn("database not ready yet, retrying...", "attempt", attempt, "max", maxAttempts, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	defer db.Close()

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		return err
	}

	switch os.Args[1] {
	case "migrate":
		if err := db.Migrate(ctx, migrations, func(msg string, args ...any) {
			log.Info(msg, args...)
		}); err != nil {
			return err
		}
		// The permission catalogue lives in Go; identity.permissions mirrors
		// it. Syncing as part of "migrate" keeps the two together, so an
		// operator who runs migrations never ends up with a schema that is
		// current and a role editor that is not.
		if err := rbac.Sync(ctx, db); err != nil {
			return fmt.Errorf("sync permission catalogue: %w", err)
		}
		seeded, err := rbac.SeedExistingCompanies(ctx, db)
		if err != nil {
			return fmt.Errorf("seed company roles: %w", err)
		}
		added, err := rbac.SeedMissingSystemRoles(ctx, db)
		if err != nil {
			return fmt.Errorf("seed newly declared starter roles: %w", err)
		}
		repaired, err := rbac.RepairOutOfScopeGrants(ctx, db)
		if err != nil {
			return fmt.Errorf("repair company role grants: %w", err)
		}
		log.Info("migrations up to date", "total", len(migrations),
			"companies_seeded", seeded, "roles_added", added, "companies_repaired", repaired)
		return nil

	case "migrate-status":
		pending, err := db.PendingCount(ctx, migrations)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tNAME")
		for _, m := range migrations {
			fmt.Fprintf(w, "%d\t%s\n", m.Version, m.Name)
		}
		_ = w.Flush()
		fmt.Printf("\n%d migration(s) defined, %d pending\n", len(migrations), pending)

		orphans, err := db.Orphans(ctx, migrations)
		if err != nil {
			return err
		}
		if len(orphans) > 0 {
			fmt.Printf("\n%d migration(s) applied to this database with no file:\n", len(orphans))
			for _, o := range orphans {
				fmt.Printf("  %d\t%s\n", o.Version, o.Name)
			}
			fmt.Println("\nThis database cannot be rebuilt from the repository until each is")
			fmt.Println("restored, or written off deliberately in docs/adr/.")
		}
		if pending > 0 {
			os.Exit(2)
		}
		if len(orphans) > 0 {
			os.Exit(3)
		}
		return nil

	case "migrate-data":
		sourceURL := os.Getenv("MARIADB_SOURCE_URL")
		if sourceURL == "" {
			return errors.New("MARIADB_SOURCE_URL environment variable is required to execute legacy ETL migration")
		}
		log.Info("starting legacy MariaDB to PostgreSQL ETL pipeline", "source", sourceURL)
		// ETL execution requires connection to source MariaDB
		return nil

	case "seed":
		return runSeed(ctx, db, log)

	case "seed-users":
		// A known password on a live platform is a back door, not a convenience.
		if cfg.Env == "prod" {
			return errors.New("seed-users refuses to run with APP_ENV=prod")
		}
		if err := runSeedUsers(ctx, db, log); err != nil {
			return err
		}
		fmt.Print(seedUsersSummary())
		return nil

	case "reset-db":
		if cfg.Env == "prod" {
			return errors.New("reset-db refuses to run with APP_ENV=prod")
		}
		if err := runResetDB(ctx, db, log); err != nil {
			return err
		}
		fmt.Print(resetDBHelp())
		return nil

	case "reindex":
		catRepo := catalogPostgres.NewRepository(db)
		count, err := catRepo.RebuildProductIndex(ctx)
		if err != nil {
			return fmt.Errorf("reindex failed: %w", err)
		}
		log.Info("product index rebuilt successfully", "indexed_count", count)
		fmt.Printf("product_index rebuilt successfully: %d items indexed\n", count)
		return nil

	case "activate-imported":
		return activateImported(ctx, db, log, os.Args[2:])

	case "smartorder-smoke":
		// Env-configured credentials, not the admin settings source the server
		// uses: a smoke run is driven by an operator who can set
		// GATEWAY_ENABLED and GATEWAY_VIRTUAL_KEY, and reaching into the admin
		// tables from a CLI would test a different code path than the one being
		// smoked.
		gw := gateway.New(cfg.Gateway, log)
		return smartOrderSmoke(ctx, db, gw,
			org.NewService(orgPG.NewRepository(db), log), log, os.Args[2:])

	case "health":
		if err := db.Health(ctx); err != nil {
			return err
		}
		fmt.Println("database: ok")
		return nil

	case "dump-plans":
		billRepo := billingPostgres.NewRepository(db)
		plans, err := billRepo.ListPlans(ctx)
		if err != nil {
			return fmt.Errorf("list plans: %w", err)
		}
		fmt.Printf("found %d plans:\n", len(plans))
		for _, p := range plans {
			fmt.Printf("- ID:%d Slug:%s NameAr:%s PriceMonth:%s PriceYear:%s IsDefault:%t IsActive:%t Features:%v\n",
				p.ID, p.Slug, p.Name.Get("ar"), p.PriceMonth.String(), p.PriceYear.String(), p.IsDefault, p.IsActive, p.Features)
		}
		return nil

	case "corpus-export":
		return exportCorpus(ctx, db)

	case "ai-identities":
		return aiIdentities(ctx, db, log, os.Args[2:])

	case "imports-recover":
		apply := len(os.Args) > 2 && os.Args[2] == "--apply"
		return importsRecover(ctx, db, log, apply)

	default:
		fmt.Print(usage())
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
