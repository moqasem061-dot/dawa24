package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The institutional works schema (org.institutional_works and
// org.institutional_work_connections) is owned by migrations 077 and 079.
// It used to be re-declared as CREATE TABLE IF NOT EXISTS at the top of every
// repository call, including the read ones — which run in a read-only
// transaction, so every institutional read failed with
// "cannot execute CREATE TABLE in a read-only transaction". That error
// propagated all the way to commerce.CheckAvailability, which fails closed, so
// the storefront hid every supplier offer from every pharmacy.

func seedDefaultInstitutionalWorks(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org.institutional_works WHERE deleted_at IS NULL;`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []struct {
		titleAr     string
		titleEn     string
		descAr      string
		descEn      string
		pricingType string
		icon        string
		viewType    int
		children    []struct {
			titleAr  string
			titleEn  string
			descAr   string
			descEn   string
			pricing  string
			icon     string
			viewType int
		}
	}{
		{
			titleAr:     i18n.T("ar", "tier.mega_wholesale_title"),
			titleEn:     "Wholesale - Wholesale",
			descAr:      i18n.T("ar", "tier.mega_wholesale_desc"),
			descEn:      "Primary large-scale pharmaceutical wholesalers and master hubs",
			pricingType: "subscription",
			icon:        "truck",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  i18n.T("ar", "tier.sector_title"),
					titleEn:  "Sector",
					descAr:   i18n.T("ar", "tier.sector_desc"),
					descEn:   "Specialized geographical distribution sector",
					pricing:  "subscription",
					icon:     "layers",
					viewType: 1,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_405_405"),
					titleEn:  "Factory",
					descAr:   i18n.TDefault("w4_mod.s_406_406"),
					descEn:   "Pharmaceutical manufacturing plants and laboratories",
					pricing:  "paid",
					icon:     "package",
					viewType: 1,
				},
			},
		},
		{
			titleAr:     i18n.TDefault("w4_mod.s_407_407"),
			titleEn:     "Retail",
			descAr:      i18n.TDefault("w4_mod.s_408_408"),
			descEn:      "Retail pharmacies and direct consumer healthcare outlets",
			pricingType: "free",
			icon:        "pill",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  i18n.TDefault("w4_ui.s_195_195"),
					titleEn:  "Pharmacy",
					descAr:   i18n.TDefault("w4_mod.s_409_409"),
					descEn:   "Licensed community dispensing pharmacy",
					pricing:  "free",
					icon:     "pill",
					viewType: 2,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_410_410"),
					titleEn:  "Audience Category",
					descAr:   i18n.TDefault("w4_mod.s_411_411"),
					descEn:   "Direct customer retail category",
					pricing:  "free",
					icon:     "cart",
					viewType: 1,
				},
			},
		},
		{
			titleAr:     i18n.TDefault("w4_mod.s_412_412"),
			titleEn:     "Services",
			descAr:      i18n.TDefault("w4_mod.s_413_413"),
			descEn:      "Pharma cold-chain logistics, technical consultancy, and enterprise services",
			pricingType: "monthly",
			icon:        "shield",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  i18n.TDefault("w4_mod.s_414_414"),
					titleEn:  "Joint-Stock Company",
					descAr:   i18n.TDefault("w4_mod.s_415_415"),
					descEn:   "Corporate joint-stock healthcare enterprises",
					pricing:  "subscription",
					icon:     "briefcase",
					viewType: 1,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_416_416"),
					titleEn:  "Sole Proprietorship",
					descAr:   i18n.TDefault("w4_mod.s_417_417"),
					descEn:   "Single proprietor medical agencies and scientific offices",
					pricing:  "paid",
					icon:     "building",
					viewType: 1,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_418_418"),
					titleEn:  "Cooperatives",
					descAr:   i18n.TDefault("w4_mod.s_419_419"),
					descEn:   "Healthcare & pharmaceutical cooperatives",
					pricing:  "per_project",
					icon:     "users",
					viewType: 1,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_420_420"),
					titleEn:  "Startups",
					descAr:   i18n.TDefault("w4_mod.s_421_421"),
					descEn:   "HealthTech and pharmacy supply chain startups",
					pricing:  "free",
					icon:     "tag",
					viewType: 1,
				},
				{
					titleAr:  i18n.TDefault("w4_mod.s_422_422"),
					titleEn:  "Non-profit Organization",
					descAr:   i18n.TDefault("w4_mod.s_423_423"),
					descEn:   "Non-profit healthcare and humanitarian medical organizations",
					pricing:  "free",
					icon:     "shield",
					viewType: 1,
				},
			},
		},
	}

	idMap := make(map[string]int64)

	for _, g := range defaults {
		var parentID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug)
			VALUES ($1, $2, $3, $4, true, $5, $6)
			RETURNING id;
		`, i18n.New(g.titleAr, g.titleEn), i18n.New(g.descAr, g.descEn), g.icon, g.pricingType, g.viewType, g.titleEn).Scan(&parentID)
		if err != nil {
			return err
		}
		idMap[g.titleEn] = parentID

		for _, c := range g.children {
			var childID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
				VALUES ($1, $2, $3, $4, true, $5, $6, $7)
				RETURNING id;
			`, i18n.New(c.titleAr, c.titleEn), i18n.New(c.descAr, c.descEn), c.icon, c.pricing, c.viewType, c.titleEn, parentID).Scan(&childID)
			if err != nil {
				return err
			}
			idMap[c.titleEn] = childID
		}
	}

	// Seed baseline connections:
	// Factory -> Wholesale, Retail
	// Wholesale -> Factory, Retail, Pharmacy
	// Pharmacy -> Wholesale
	// Services -> Wholesale
	defaultConnections := [][2]string{
		{"Factory", "Wholesale - Wholesale"},
		{"Factory", "Retail"},
		{"Wholesale - Wholesale", "Factory"},
		{"Wholesale - Wholesale", "Retail"},
		{"Wholesale - Wholesale", "Pharmacy"},
		{"Pharmacy", "Wholesale - Wholesale"},
		{"Services", "Wholesale - Wholesale"},
		{"Sector", "Factory"},
		{"Sector", "Wholesale - Wholesale"},
	}

	for _, pair := range defaultConnections {
		fromID, ok1 := idMap[pair[0]]
		toID, ok2 := idMap[pair[1]]
		if ok1 && ok2 && fromID > 0 && toID > 0 {
			_, _ = tx.Exec(ctx, `
				INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
				VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
			`, fromID, toID)
		}
	}

	return nil
}

// CreateInstitutionalWork creates a new institutional structure category and syncs allowed connections.
func (r *Repository) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var parentID *int64
		if iw.ParentID != nil && *iw.ParentID > 0 {
			parentID = iw.ParentID
		}
		const query = `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, parentID,
		).Scan(&iw.ID, &iw.PublicID, &iw.CreatedAt, &iw.UpdatedAt); err != nil {
			return err
		}

		// Sync connections
		if len(iw.AllowedConnections) > 0 {
			for _, toID := range iw.AllowedConnections {
				if toID > 0 && toID != iw.ID {
					if _, err := tx.Exec(txCtx, `
						INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
						VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
					`, iw.ID, toID); err != nil {
						return err
					}
				}
			}
		}

		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			Action:     "reference.institutional_work.create",
			EntityType: "institutional_work",
			EntityID:   strconv.FormatInt(iw.ID, 10),
			After:      map[string]any{"title": iw.Title, "pricing_type": iw.PricingType},
		})

		return nil
	})
}
