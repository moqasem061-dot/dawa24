package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements promo.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

var _ promo.Repository = (*Repository)(nil)

// NewRepository creates a new promo PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateOffer inserts a promotion campaign for the active organization.
func (r *Repository) CreateOffer(ctx context.Context, o *promo.Offer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offers (
				organization_id, title, description, discount_type, discount_value,
				branch_id, admin_status, min_order_amount,
				starts_at, expires_at, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		if o.AdminStatus == "" {
			o.AdminStatus = "pending"
		}
		err := tx.QueryRow(txCtx, query,
			o.OrganizationID, o.Title, o.Description, string(o.DiscountType),
			o.DiscountValue, o.BranchID, o.AdminStatus,
			o.MinOrderAmount, o.StartsAt, o.ExpiresAt, o.IsActive,
		).Scan(&o.ID, &o.PublicID, &o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return fmt.Errorf("promo postgres: create offer: %w", err)
		}

		for _, prodID := range o.ProductIDs {
			_, err := tx.Exec(txCtx, `INSERT INTO promo.offer_products (offer_id, product_id) VALUES ($1, $2);`, o.ID, prodID)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetOfferByID retrieves an offer and its mapped product IDs.
func (r *Repository) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	var offer *promo.Offer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		offer, err = scanOffer(tx.QueryRow(txCtx, `
			SELECT `+offerColumns+`
			FROM promo.offers
			WHERE id = $1 AND deleted_at IS NULL;
		`, id))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("offer")
			}
			return err
		}

		rows, err := tx.Query(txCtx, `SELECT product_id FROM promo.offer_products WHERE offer_id = $1;`, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pID int64
			if err := rows.Scan(&pID); err != nil {
				return err
			}
			offer.ProductIDs = append(offer.ProductIDs, pID)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return offer, nil
}

// ListActiveOffers returns all approved offers currently running across vendors.
func (r *Repository) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	var offers []*promo.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rule := &offerRule{}
		query := `
			SELECT ` + offerColumnsAs("o") + `
			FROM promo.offers o
			WHERE ` + rule.live() + ` AND ` + rule.supplier() + ` AND ` + rule.branch() + `
			ORDER BY o.id DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOffer(rows)
			if err != nil {
				return err
			}
			offers = append(offers, o)
		}
		return rows.Err()
	})
	return offers, err
}

// IncrementOfferEngagement atomically records views or clicks for an offer.
func (r *Repository) IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE promo.offers SET views_count = views_count + 1 WHERE id = $1;`
		if isClick {
			query = `UPDATE promo.offers SET clicks_count = clicks_count + 1 WHERE id = $1;`
		}
		_, err := tx.Exec(txCtx, query, offerID)
		return err
	})
}

// CreatePackage inserts a sponsorship package.
func (r *Repository) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offer_packages (name, description, price, duration_days, max_offers, credits, tier_level, sort_order, is_featured, badge_color, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		if p.Credits <= 0 {
			p.Credits = p.MaxOffers
		}
		if p.TierLevel <= 0 {
			p.TierLevel = 1
		}
		if p.BadgeColor == "" {
			p.BadgeColor = "#0284c7"
		}
		return tx.QueryRow(txCtx, query,
			p.Name, p.Description, p.Price, p.DurationDays, p.MaxOffers, p.Credits,
			p.TierLevel, p.SortOrder, p.IsFeatured, p.BadgeColor, p.IsActive,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// UpdatePackage updates a sponsorship package.
func (r *Repository) UpdatePackage(ctx context.Context, p *promo.OfferPackage) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.offer_packages SET
				name = $2, description = $3, price = $4, duration_days = $5, max_offers = $6,
				credits = $7, tier_level = $8, sort_order = $9, is_featured = $10,
				badge_color = $11, is_active = $12, updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query,
			p.ID, p.Name, p.Description, p.Price, p.DurationDays, p.MaxOffers,
			p.Credits, p.TierLevel, p.SortOrder, p.IsFeatured, p.BadgeColor, p.IsActive,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("offer_package")
		}
		return nil
	})
}

// GetPackageByID retrieves a sponsorship package by ID.
func (r *Repository) GetPackageByID(ctx context.Context, id int64) (*promo.OfferPackage, error) {
	var p promo.OfferPackage
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return scanPackage(tx.QueryRow(txCtx, `SELECT `+packageColumns+` FROM promo.offer_packages WHERE id = $1;`, id), &p)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("offer_package")
		}
		return nil, err
	}
	return &p, nil
}

// ListPackages returns all active packages.
func (r *Repository) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return r.listPackages(ctx, true)
}

// AdminListPackages returns all packages (active and inactive) for admin management.
func (r *Repository) AdminListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return r.listPackages(database.AsSystem(ctx), false)
}

func (r *Repository) listPackages(ctx context.Context, activeOnly bool) ([]*promo.OfferPackage, error) {
	var list []*promo.OfferPackage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + packageColumns + ` FROM promo.offer_packages`
		if activeOnly {
			query += ` WHERE is_active = true`
		}
		query += ` ORDER BY tier_level DESC, sort_order ASC, id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p promo.OfferPackage
			if err := scanPackage(rows, &p); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// TogglePackageActive activates or deactivates a package.
func (r *Repository) TogglePackageActive(ctx context.Context, id int64, active bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE promo.offer_packages SET is_active = $2, updated_at = now() WHERE id = $1;`, id, active)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("offer_package")
		}
		return nil
	})
}

const packageColumns = `id, public_id, name, description, price, duration_days, max_offers, credits, tier_level, sort_order, is_featured, badge_color, is_active, created_at, updated_at`

func scanPackage(row pgx.Row, p *promo.OfferPackage) error {
	return row.Scan(
		&p.ID, &p.PublicID, &p.Name, &p.Description, &p.Price, &p.DurationDays, &p.MaxOffers,
		&p.Credits, &p.TierLevel, &p.SortOrder, &p.IsFeatured, &p.BadgeColor, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
}

// CreateSponsorship links an offer to a paid package.
func (r *Repository) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		itemType := string(s.ItemType)
		if itemType == "" {
			itemType = "offer"
		}
		itemID := s.ItemID
		if itemID == 0 {
			itemID = s.OfferID
		}
		query := `
			INSERT INTO promo.offer_sponsorships (organization_id, offer_id, package_id, starts_at, expires_at, status, item_type, item_id, credits_used, admin_status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at;
		`
		adminStatus := string(s.AdminStatus)
		if adminStatus == "" {
			adminStatus = "approved"
		}
		return tx.QueryRow(txCtx, query,
			s.OrganizationID, s.OfferID, s.PackageID, s.StartsAt, s.ExpiresAt, s.Status,
			itemType, itemID, s.CreditsUsed, adminStatus,
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt)
	})
}

// ListActiveAds is implemented in ads.go.

// RecordAdClick logs click analytics and increments counters.
func (r *Repository) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE promo.ads SET clicks = clicks + 1 WHERE id = $1;`, adID); err != nil {
			return err
		}
		query := `INSERT INTO promo.ad_clicks (ad_id, user_id, ip_address, user_agent) VALUES ($1, $2, $3, $4);`
		_, err := tx.Exec(txCtx, query, adID, userID, ip, ua)
		return err
	})
}

// CreateHighlightSection creates a homepage curated section. platform rows
// are created via the promo API; organization rows carry owner_type +
// organization_id (066).
func (r *Repository) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.highlight_sections (title, description, section_type, color, slug, display_order, is_active, show_in_header, owner_type, organization_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		if h.OwnerType == "" {
			h.OwnerType = "platform"
		}
		if h.SectionType == "" {
			h.SectionType = "about"
		}
		if h.Color == "" {
			h.Color = "#0284c7"
		}
		return tx.QueryRow(txCtx, query,
			h.Title, h.Description, h.SectionType, h.Color,
			h.Slug, h.DisplayOrder, h.IsActive, h.ShowInHeader,
			h.OwnerType, h.OrganizationID,
		).Scan(&h.ID, &h.PublicID, &h.CreatedAt, &h.UpdatedAt)
	})
}
