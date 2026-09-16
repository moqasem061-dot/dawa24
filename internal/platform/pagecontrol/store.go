package pagecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

// Store reads and writes platform_admin.managed_pages.
type Store struct {
	db *database.DB
}

// NewStore builds a store over a database handle. A nil db yields a store whose
// reads return empty and whose writes error — the engine treats that as
// fail-open, which is the same posture features takes when its table is absent.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// Actor attributes a write. RequestID is optional and only used for the audit row.
type Actor struct {
	UserID    int64
	RequestID string
}

// Filter narrows a List call.
type Filter struct {
	Resource Resource // empty = every resource
}

// CreateInput is a manually added page.
type CreateInput struct {
	Path        string
	MatchMode   MatchMode
	Resource    Resource // empty = inferred from the path
	LabelAr     string
	LabelEn     string
	Description string
}

const pageColumns = `id, resource, path, match_mode, label, description,
	is_enabled, is_system, is_lockable, route_patterns, source,
	discovered_at, updated_by, updated_at, created_at`

// List returns every non-deleted page, newest change first, optionally for one
// resource.
func (s *Store) List(ctx context.Context, f Filter) ([]Page, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var out []Page
	err := s.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		q := `SELECT ` + pageColumns + ` FROM platform_admin.managed_pages WHERE deleted_at IS NULL`
		args := []any{}
		if f.Resource != "" {
			q += ` AND resource = $1`
			args = append(args, string(f.Resource))
		}
		q += ` ORDER BY resource, path`
		rows, err := tx.Query(txCtx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanPage(rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// Get returns one page by id.
func (s *Store) Get(ctx context.Context, id int64) (Page, error) {
	var p Page
	err := s.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+pageColumns+` FROM platform_admin.managed_pages WHERE id = $1 AND deleted_at IS NULL`, id)
		var err error
		p, err = scanPage(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, fmt.Errorf("managed page %d not found", id)
	}
	return p, err
}

// Snapshot returns the compact matcher view of every non-deleted page.
func (s *Store) Snapshot(ctx context.Context) ([]rule, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var out []rule
	err := s.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx,
			`SELECT id, path, match_mode, is_enabled, label, COALESCE(description, '') FROM platform_admin.managed_pages WHERE deleted_at IS NULL`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r rule
			var mode string
			var labelJSON []byte
			var desc string
			if err := rows.Scan(&r.id, &r.path, &mode, &r.enabled, &labelJSON, &desc); err != nil {
				return err
			}
			r.path = NormalizePath(r.path)
			r.mode = MatchMode(mode)
			r.description = desc
			if len(labelJSON) > 0 {
				var m map[string]string
				if json.Unmarshal(labelJSON, &m) == nil {
					r.labelAr, r.labelEn = m["ar"], m["en"]
				}
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// Version reads the cross-process invalidation counter.
func (s *Store) Version(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var v int64
	err := s.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT version FROM platform_admin.page_control_version WHERE scope_key = 'global'`).Scan(&v)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// --- helpers shared with store_writes.go ---

type scanner interface {
	Scan(dest ...any) error
}

func scanPage(row scanner) (Page, error) {
	var p Page
	var res, mode, src string
	var labelJSON []byte
	if err := row.Scan(
		&p.ID, &res, &p.Path, &mode, &labelJSON, &p.Description,
		&p.IsEnabled, &p.IsSystem, &p.IsLockable, &p.RoutePatterns, &src,
		&p.DiscoveredAt, &p.UpdatedBy, &p.UpdatedAt, &p.CreatedAt,
	); err != nil {
		return Page{}, err
	}
	p.Resource = Resource(res)
	p.MatchMode = MatchMode(mode)
	p.Source = Source(src)
	if len(labelJSON) > 0 {
		var m map[string]string
		if json.Unmarshal(labelJSON, &m) == nil {
			p.LabelAr, p.LabelEn = m["ar"], m["en"]
		}
	}
	return p, nil
}

// RequestIDFrom re-exports observability.RequestIDFrom so a caller building an
// Actor need not import observability for one string.
func RequestIDFrom(ctx context.Context) string { return observability.RequestIDFrom(ctx) }
