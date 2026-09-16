package postgres

import (
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements commerce.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new PostgreSQL commerce repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}
