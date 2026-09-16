package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// Pipeline orchestrates the MariaDB to PostgreSQL migration.
type Pipeline struct {
	sourceDSN   string
	targetDSN   string
	batchSize   int
	validator   *Validator
	transformer *Transformer
	log         *slog.Logger
}

// NewPipeline creates a new ETL migration pipeline.
func NewPipeline(sourceDSN, targetDSN string, batchSize int, log *slog.Logger) *Pipeline {
	return &Pipeline{
		sourceDSN:   sourceDSN,
		targetDSN:   targetDSN,
		batchSize:   batchSize,
		validator:   NewValidator(),
		transformer: NewTransformer(),
		log:         log,
	}
}

// Run executes the migration.
func (p *Pipeline) Run(ctx context.Context, verifyOnly bool) error {
	p.log.Info("connecting to databases")
	sourceDB, err := sql.Open("mysql", p.sourceDSN)
	if err != nil {
		return fmt.Errorf("source db: %w", err)
	}
	defer sourceDB.Close()

	targetDB, err := sql.Open("pgx", p.targetDSN)
	if err != nil {
		return fmt.Errorf("target db: %w", err)
	}
	defer targetDB.Close()

	if verifyOnly {
		p.log.Info("running verification gates only")
		return p.Verify(ctx, sourceDB, targetDB)
	}

	extractor := NewExtractor(sourceDB)
	loader := NewLoader(targetDB)

	// Stage 1: Users
	p.log.Info("starting users migration stage")
	if err := p.migrateUsers(ctx, extractor, loader); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	// Stage 2: Organizations / Suppliers
	p.log.Info("starting organizations migration stage")
	if err := p.migrateOrganizations(ctx, extractor, loader); err != nil {
		p.log.Warn("organization migration warning (table may differ)", "error", err)
	}

	// Stage 3: Products & Variants
	p.log.Info("starting products migration stage")
	if err := p.migrateProducts(ctx, extractor, loader); err != nil {
		p.log.Warn("products migration warning (table may differ)", "error", err)
	}

	p.log.Info("running post-migration verification gates")
	return p.Verify(ctx, sourceDB, targetDB)
}

func (p *Pipeline) migrateUsers(ctx context.Context, extractor *Extractor, loader *Loader) error {
	offset := 0
	totalLoaded := 0

	for {
		users, err := extractor.ExtractUsers(ctx, p.batchSize, offset)
		if err != nil {
			return fmt.Errorf("extract users batch at offset %d: %w", offset, err)
		}
		if len(users) == 0 {
			break
		}

		var targetUsers []*TargetUser
		for _, u := range users {
			if err := p.validator.ValidateUser(u); err != nil {
				p.log.Warn("skipping invalid user", "id", u.ID, "error", err)
				continue
			}
			targetUsers = append(targetUsers, p.transformer.TransformUser(u))
		}

		loaded, err := loader.LoadUsers(ctx, targetUsers)
		if err != nil {
			return fmt.Errorf("load users batch: %w", err)
		}
		totalLoaded += loaded
		offset += len(users)
	}

	p.log.Info("users migration completed", "total_loaded", totalLoaded)
	return nil
}

func (p *Pipeline) migrateOrganizations(ctx context.Context, extractor *Extractor, loader *Loader) error {
	offset := 0
	totalLoaded := 0

	for {
		orgs, err := extractor.ExtractOrganizations(ctx, p.batchSize, offset)
		if err != nil {
			return err
		}
		if len(orgs) == 0 {
			break
		}

		var targetOrgs []*TargetOrg
		for _, o := range orgs {
			targetOrgs = append(targetOrgs, p.transformer.TransformOrg(o))
		}

		loaded, err := loader.LoadOrganizations(ctx, targetOrgs)
		if err != nil {
			return fmt.Errorf("load orgs batch: %w", err)
		}
		totalLoaded += loaded
		offset += len(orgs)
	}

	p.log.Info("organizations migration completed", "total_loaded", totalLoaded)
	return nil
}

func (p *Pipeline) migrateProducts(ctx context.Context, extractor *Extractor, loader *Loader) error {
	offset := 0
	totalLoaded := 0

	for {
		products, err := extractor.ExtractProducts(ctx, p.batchSize, offset)
		if err != nil {
			return err
		}
		if len(products) == 0 {
			break
		}

		var targetProducts []*TargetProduct
		var targetVariants []*TargetVariant
		for _, prod := range products {
			if err := p.validator.ValidateProduct(prod); err != nil {
				p.log.Warn("skipping invalid product", "id", prod.ID, "error", err)
				continue
			}
			tProd, tVar := p.transformer.TransformProduct(prod)
			targetProducts = append(targetProducts, tProd)
			targetVariants = append(targetVariants, tVar)
		}

		loaded, err := loader.LoadProducts(ctx, targetProducts, targetVariants)
		if err != nil {
			return fmt.Errorf("load products batch: %w", err)
		}
		totalLoaded += loaded
		offset += len(products)
	}

	p.log.Info("products migration completed", "total_loaded", totalLoaded)
	return nil
}

// Verify runs 2-way verification gates between source and target databases.
func (p *Pipeline) Verify(ctx context.Context, source *sql.DB, target *sql.DB) error {
	var srcUsers, tgtUsers int
	if err := source.QueryRowContext(ctx, `SELECT COUNT(*) FROM users;`).Scan(&srcUsers); err != nil {
		return fmt.Errorf("verify source users count: %w", err)
	}
	if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM identity.users;`).Scan(&tgtUsers); err != nil {
		return fmt.Errorf("verify target users count: %w", err)
	}

	p.log.Info("verification gate results",
		"source_users", srcUsers,
		"target_users", tgtUsers,
	)

	if srcUsers > tgtUsers {
		return fmt.Errorf("verification gate failed: target users (%d) < source users (%d)", tgtUsers, srcUsers)
	}
	return nil
}
