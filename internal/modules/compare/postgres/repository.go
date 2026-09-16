package postgres

import (
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements compare.Repository backed by PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new compare repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

var _ compare.Repository = (*Repository)(nil)
