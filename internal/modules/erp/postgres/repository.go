package postgres

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

type Repository struct{ db *database.DB }

func New(db *database.DB) *Repository { return &Repository{db: db} }

// interfaces: the Postgres repository implements the whole erp port (T118).
var _ erp.Repository = (*Repository)(nil)
