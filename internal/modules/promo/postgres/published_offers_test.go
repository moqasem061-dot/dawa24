package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestListPublishedOffers checks the announcement feed follows the buyer rule:
// only live offers of approved suppliers on live branches, with products,
// published inside the window.
func TestListPublishedOffers(t *testing.T) {
	db := getTestDB(t)
	ctx := database.AsSystem(context.Background())
	pool := db.Pool()
	f := seedOfferRule(t, ctx, db)

	var product int64
	if err := pool.QueryRow(ctx, `SELECT id FROM catalog.products WHERE deleted_at IS NULL ORDER BY id LIMIT 1`).Scan(&product); err != nil {
		t.Skip("needs a seeded product")
	}
	withProducts := []string{
		"weekly coverage, no branch", "weekly coverage, live branch", "deleted supplier branch",
		"own rule, city, today", "unapproved supplier", "pending approval", "expired",
	}
	for _, name := range withProducts {
		if _, err := pool.Exec(ctx, `INSERT INTO promo.offer_products (offer_id, product_id) VALUES ($1, $2)`, f.offers[name], product); err != nil {
			t.Fatal(err)
		}
	}
	// Published three days ago: live, but old news.
	if _, err := pool.Exec(ctx, `UPDATE promo.offers SET created_at = now() - interval '3 days', approved_at = now() - interval '3 days'
		WHERE id = $1`, f.offers["weekly coverage, live branch"]); err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)
	got, err := repo.ListPublishedOffers(ctx, time.Now().Add(-24*time.Hour), 1000)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string][]string{}
	for _, o := range got {
		if name, ok := strings.CutPrefix(o.TitleAr, f.mark+" "); ok {
			listed[name] = o.Cities
		}
	}

	want := []string{"weekly coverage, no branch", "own rule, city, today"}
	if len(listed) != len(want) {
		t.Fatalf("listed %v, want exactly %v", listed, want)
	}
	for _, name := range want {
		if _, ok := listed[name]; !ok {
			t.Errorf("%q missing from %v", name, listed)
		}
	}
	if cities := listed["own rule, city, today"]; len(cities) != 1 {
		t.Errorf("offer with its own city rule should name that one city, got %v", cities)
	}
}
