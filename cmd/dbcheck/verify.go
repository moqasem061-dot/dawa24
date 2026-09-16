package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// verify reports the schema state that matters for correctness: how many tables
// exist, and — critically — whether every tenant-owned table has both
// rowsecurity and forcerowsecurity. FORCE is what stops a table owner from
// bypassing every policy, and the application connects as the owner.
func verify(dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	var tables int
	_ = conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog','information_schema','public')
		  AND table_type = 'BASE TABLE'`).Scan(&tables)
	fmt.Println("tables in module schemas:", tables)

	fmt.Println("\nrow-level security on tenant-owned schemas:")
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND n.nspname IN ('org','catalog','inventory','commerce','promo','billing','ingest','workflow','hr')
		ORDER BY c.relrowsecurity, n.nspname, c.relname`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rls query: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	var total, secured, unsecured int
	var gaps []string
	for rows.Next() {
		var schema, table string
		var rls, force bool
		if err := rows.Scan(&schema, &table, &rls, &force); err != nil {
			break
		}
		total++
		if rls && force {
			secured++
		} else {
			unsecured++
			gaps = append(gaps, fmt.Sprintf("  %s.%-32s rls=%-5t force=%-5t", schema, table, rls, force))
		}
	}
	fmt.Printf("  %d/%d tables have ENABLE + FORCE\n", secured, total)
	if len(gaps) > 0 {
		fmt.Printf("\n  %d WITHOUT full protection:\n", unsecured)
		for _, g := range gaps {
			fmt.Println(g)
		}
	}

	// Columns migration 027 and 028 were written to add.
	fmt.Println("\ncolumns added by 027/028:")
	checks := []struct{ table, col string }{
		{"identity.user_addresses", "recipient"},
		{"identity.user_addresses", "address"},
		{"identity.user_addresses", "apartment"},
		{"org.organizations", "legal_name"},
		{"org.organizations", "commercial_register"},
		{"org.branches", "code"},
		{"org.members", "is_active"},
		{"commerce.orders", "rating"},
		{"commerce.orders", "rated_at"},
	}
	for _, c := range checks {
		var exists bool
		_ = conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = split_part($1,'.',1)
				  AND table_name  = split_part($1,'.',2)
				  AND column_name = $2)`, c.table, c.col).Scan(&exists)
		mark := "ok "
		if !exists {
			mark = "MISSING"
		}
		fmt.Printf("  %-7s %s.%s\n", mark, c.table, c.col)
	}
}
