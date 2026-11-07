package postgres

import "github.com/moqasem061-dot/dawa24/internal/platform/database"

type Repository struct{ db *database.DB }

func New(db *database.DB) *Repository { return &Repository{db: db} }
