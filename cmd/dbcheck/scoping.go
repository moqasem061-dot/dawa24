package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// scoping reports, for every table without row-level security, which column it
// could be scoped by. A table with organization_id and no policy is a tenant
// leak; a table with neither is probably platform-level reference data and
// legitimately unprotected.
func scoping(dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT n.nspname || '.' || c.relname AS tbl,
		       bool_or(a.attname = 'organization_id') AS has_org,
		       bool_or(a.attname = 'user_id')         AS has_user,
		       bool_or(a.attname = 'customer_id')     AS has_customer
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relkind = 'r' AND NOT c.relrowsecurity
		  AND n.nspname IN ('org','catalog','inventory','commerce','promo','billing','ingest','workflow','hr')
		GROUP BY 1
		ORDER BY 1`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scoping query: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Println("unprotected tables and their available scoping column:")
	for rows.Next() {
		var tbl string
		var hasOrg, hasUser, hasCustomer bool
		if err := rows.Scan(&tbl, &hasOrg, &hasUser, &hasCustomer); err != nil {
			break
		}
		scope := "none — platform-level?"
		switch {
		case hasOrg:
			scope = "organization_id  <-- TENANT LEAK"
		case hasCustomer:
			scope = "customer_id"
		case hasUser:
			scope = "user_id"
		}
		fmt.Printf("  %-42s %s\n", tbl, scope)
	}
}
