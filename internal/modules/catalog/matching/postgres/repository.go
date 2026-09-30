package postgres

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// Repository implements matching.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL matching repository.
func NewRepository(db *database.DB) matching.Repository {
	return &Repository{db: db}
}

// NewCuratedRepository creates a PostgreSQL curated facts and pack relationships repository.
func NewCuratedRepository(db *database.DB) matching.CuratedRepository {
	return &Repository{db: db}
}
