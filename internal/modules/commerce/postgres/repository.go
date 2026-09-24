package postgres

import (
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// Repository implements commerce.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new PostgreSQL commerce repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}
