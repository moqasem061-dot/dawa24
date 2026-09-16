package pagecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// This file holds the mutating half of Store. The reads live in store.go.

// Create inserts a manual page.
func (s *Store) Create(ctx context.Context, in CreateInput, a Actor) (Page, error) {
	if s == nil || s.db == nil {
		return Page{}, fmt.Errorf("pagecontrol: store is not configured")
	}
	path := NormalizePath(in.Path)
	if err := ValidatePath(path); err != nil {
		return Page{}, err
	}
	mode := in.MatchMode
	if mode == "" {
		mode = MatchExact
	}
	if !ValidMatchMode(mode) {
		return Page{}, fmt.Errorf("unknown match mode %q", mode)
	}
	if err := ValidatePrefixRule(path, mode); err != nil {
		return Page{}, err
	}
	res := in.Resource
	if res == "" {
		res = ClassifyResource(path)
	}
	if !ValidResource(res) {
		return Page{}, fmt.Errorf("unknown resource %q", res)
	}
	label, _ := json.Marshal(map[string]string{"ar": strings.TrimSpace(in.LabelAr), "en": strings.TrimSpace(in.LabelEn)})

	var created Page
	err := s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(txCtx,
			`SELECT EXISTS (SELECT 1 FROM platform_admin.managed_pages WHERE path = $1 AND deleted_at IS NULL)`,
			path).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("a page for %q already exists", path)
		}
		row := tx.QueryRow(txCtx, `
			INSERT INTO platform_admin.managed_pages
				(resource, path, match_mode, label, description, source, created_by, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'manual', $6, $6, now())
			RETURNING `+pageColumns, string(res), path, string(mode), label,
			strings.TrimSpace(in.Description), nullableID(a.UserID))
		var err error
		created, err = scanPage(row)
		if err != nil {
			return err
		}
		if err := writeAudit(txCtx, tx, a, "page_control.create", created.ID, nil, created); err != nil {
			return err
		}
		return bumpVersion(txCtx, tx)
	})
	return created, err
}

// SetEnabled flips one page. Disabling a non-lockable page is refused.
func (s *Store) SetEnabled(ctx context.Context, id int64, enabled bool, a Actor) (Page, error) {
	if s == nil || s.db == nil {
		return Page{}, fmt.Errorf("pagecontrol: store is not configured")
	}
	var updated Page
	err := s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		before, err := scanPage(tx.QueryRow(txCtx,
			`SELECT `+pageColumns+` FROM platform_admin.managed_pages WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("managed page %d not found", id)
		}
		if err != nil {
			return err
		}
		if !enabled && !before.IsLockable {
			return fmt.Errorf("%q is a protected page and cannot be disabled", before.Path)
		}
		if before.IsEnabled == enabled {
			updated = before
			return nil
		}
		row := tx.QueryRow(txCtx, `
			UPDATE platform_admin.managed_pages
			   SET is_enabled = $2, updated_by = $3, updated_at = now()
			 WHERE id = $1
			RETURNING `+pageColumns, id, enabled, nullableID(a.UserID))
		if updated, err = scanPage(row); err != nil {
			return err
		}
		if err := writeAudit(txCtx, tx, a, "page_control.toggle", id, before, updated); err != nil {
			return err
		}
		return bumpVersion(txCtx, tx)
	})
	return updated, err
}

// UpdateMeta edits the operator-facing label and description of a page.
func (s *Store) UpdateMeta(ctx context.Context, id int64, labelAr, labelEn, desc string, a Actor) (Page, error) {
	if s == nil || s.db == nil {
		return Page{}, fmt.Errorf("pagecontrol: store is not configured")
	}
	label, _ := json.Marshal(map[string]string{"ar": strings.TrimSpace(labelAr), "en": strings.TrimSpace(labelEn)})
	var updated Page
	err := s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		before, err := scanPage(tx.QueryRow(txCtx,
			`SELECT `+pageColumns+` FROM platform_admin.managed_pages WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("managed page %d not found", id)
		}
		if err != nil {
			return err
		}
		row := tx.QueryRow(txCtx, `
			UPDATE platform_admin.managed_pages
			   SET label = $2, description = $3, updated_by = $4, updated_at = now()
			 WHERE id = $1
			RETURNING `+pageColumns, id, label, strings.TrimSpace(desc), nullableID(a.UserID))
		if updated, err = scanPage(row); err != nil {
			return err
		}
		if err := writeAudit(txCtx, tx, a, "page_control.update", id, before, updated); err != nil {
			return err
		}
		return bumpVersion(txCtx, tx)
	})
	return updated, err
}

// UpdateInput specifies updates to a managed page.
type UpdateInput struct {
	Path        string
	MatchMode   MatchMode
	Resource    Resource
	LabelAr     string
	LabelEn     string
	Description string
}

// Update edits a managed page's metadata, match mode, resource, and (for manual pages) path.
func (s *Store) Update(ctx context.Context, id int64, in UpdateInput, a Actor) (Page, error) {
	if s == nil || s.db == nil {
		return Page{}, fmt.Errorf("pagecontrol: store is not configured")
	}
	mode := in.MatchMode
	if mode == "" {
		mode = MatchExact
	}
	if !ValidMatchMode(mode) {
		return Page{}, fmt.Errorf("unknown match mode %q", mode)
	}
	res := in.Resource
	if res != "" && !ValidResource(res) {
		return Page{}, fmt.Errorf("unknown resource %q", res)
	}
	label, _ := json.Marshal(map[string]string{"ar": strings.TrimSpace(in.LabelAr), "en": strings.TrimSpace(in.LabelEn)})

	var updated Page
	err := s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		before, err := scanPage(tx.QueryRow(txCtx,
			`SELECT `+pageColumns+` FROM platform_admin.managed_pages WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("managed page %d not found", id)
		}
		if err != nil {
			return err
		}

		path := before.Path
		if before.Source == SourceManual && strings.TrimSpace(in.Path) != "" {
			newPath := NormalizePath(in.Path)
			if err := ValidatePath(newPath); err != nil {
				return err
			}
			if err := ValidatePrefixRule(newPath, mode); err != nil {
				return err
			}
			if newPath != before.Path {
				var exists bool
				if err := tx.QueryRow(txCtx,
					`SELECT EXISTS (SELECT 1 FROM platform_admin.managed_pages WHERE path = $1 AND id != $2 AND deleted_at IS NULL)`,
					newPath, id).Scan(&exists); err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("a page for %q already exists", newPath)
				}
				path = newPath
			}
		}

		if res == "" {
			res = ClassifyResource(path)
		}

		row := tx.QueryRow(txCtx, `
			UPDATE platform_admin.managed_pages
			   SET path = $2,
			       match_mode = $3,
			       resource = $4,
			       label = $5,
			       description = $6,
			       updated_by = $7,
			       updated_at = now()
			 WHERE id = $1
			RETURNING `+pageColumns, id, path, string(mode), string(res), label, strings.TrimSpace(in.Description), nullableID(a.UserID))
		if updated, err = scanPage(row); err != nil {
			return err
		}
		if err := writeAudit(txCtx, tx, a, "page_control.update", id, before, updated); err != nil {
			return err
		}
		return bumpVersion(txCtx, tx)
	})
	return updated, err
}

// Delete soft-removes a manual page. Discovered and system rows stay.
func (s *Store) Delete(ctx context.Context, id int64, a Actor) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("pagecontrol: store is not configured")
	}
	return s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		before, err := scanPage(tx.QueryRow(txCtx,
			`SELECT `+pageColumns+` FROM platform_admin.managed_pages WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("managed page %d not found", id)
		}
		if err != nil {
			return err
		}
		if before.IsSystem || before.Source != SourceManual {
			return fmt.Errorf("%q was discovered from the route table and cannot be deleted; disable it instead", before.Path)
		}
		if _, err := tx.Exec(txCtx,
			`UPDATE platform_admin.managed_pages SET deleted_at = now(), updated_by = $2 WHERE id = $1`,
			id, nullableID(a.UserID)); err != nil {
			return err
		}
		if err := writeAudit(txCtx, tx, a, "page_control.delete", id, before, nil); err != nil {
			return err
		}
		return bumpVersion(txCtx, tx)
	})
}

// UpsertDiscovered inserts rows for candidates not already present and refreshes
// route_patterns on the ones that are. It never touches is_enabled, label or
// is_lockable: an operator's decision outlives a redeploy.
func (s *Store) UpsertDiscovered(ctx context.Context, cands []Candidate) (int, error) {
	if s == nil || s.db == nil || len(cands) == 0 {
		return 0, nil
	}
	added := 0
	err := s.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for _, c := range cands {
			path := NormalizePath(c.Path)
			if ValidatePath(path) != nil {
				continue
			}
			res := c.Resource
			if res == "" {
				res = ClassifyResource(path)
			}
			tag, err := tx.Exec(txCtx, `
				INSERT INTO platform_admin.managed_pages
					(resource, path, match_mode, label, route_patterns, source, discovered_at, updated_at)
				VALUES ($1, $2, 'prefix', $3, $4, 'discovered', now(), now())
				ON CONFLICT (path) WHERE deleted_at IS NULL DO UPDATE
				   SET route_patterns = EXCLUDED.route_patterns,
				       discovered_at  = now()`,
				string(res), path,
				mustLabel(c.LabelAr, c.LabelEn),
				c.Patterns)
			if err != nil {
				return err
			}
			if tag.Insert() {
				added++
			}
		}
		if added > 0 {
			return bumpVersion(txCtx, tx)
		}
		return nil
	})
	return added, err
}

func bumpVersion(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT platform_admin.bump_page_control_version()`)
	return err
}

func writeAudit(ctx context.Context, tx pgx.Tx, a Actor, action string, id int64, before, after any) error {
	return database.WriteAudit(ctx, tx, database.AuditEntry{
		ActorUserID: a.UserID,
		Action:      action,
		EntityType:  "managed_page",
		EntityID:    fmt.Sprintf("%d", id),
		Before:      before,
		After:       after,
		RequestID:   a.RequestID,
	})
}

func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func mustLabel(ar, en string) []byte {
	b, _ := json.Marshal(map[string]string{"ar": ar, "en": en})
	return b
}
