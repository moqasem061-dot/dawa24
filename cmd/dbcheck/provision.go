package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// provision creates the least-privilege application role and grants it what the
// application needs.
//
// The role is deliberately NOT a superuser and does NOT own the tables. A
// superuser bypasses row-level security unconditionally — even FORCE does not
// apply to it — so an application connecting as one has no tenant isolation at
// all, however many policies exist.
func provision(adminDSN, roleName, password string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	ident := pgx.Identifier{roleName}.Sanitize()
	lit := quoteLiteral(password)

	var exists bool
	_ = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, roleName).Scan(&exists)

	if exists {
		if _, err := conn.Exec(ctx, fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD %s", ident, lit)); err != nil {
			fmt.Fprintf(os.Stderr, "alter role: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("role %s already existed; password reset\n", roleName)
	} else {
		stmt := fmt.Sprintf(
			`CREATE ROLE %s WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 40`,
			ident, lit)
		if _, err := conn.Exec(ctx, stmt); err != nil {
			fmt.Fprintf(os.Stderr, "create role: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("created role %s\n", roleName)
	}

	schemas := []string{
		"identity", "profile", "org", "catalog", "inventory", "commerce",
		"promo", "billing", "ingest", "workflow", "hr", "platform",
		"platform_admin", "notifications", "ai", "public",
	}

	for _, s := range schemas {
		var present bool
		_ = conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`, s).Scan(&present)
		if !present {
			continue
		}
		si := pgx.Identifier{s}.Sanitize()
		stmts := []string{
			fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", si, ident),
			fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO %s", si, ident),
			fmt.Sprintf("GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %s TO %s", si, ident),
			fmt.Sprintf("GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA %s TO %s", si, ident),
			// Objects created by future migrations inherit these, so this does
			// not have to be re-run for every new table.
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s", si, ident),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT USAGE, SELECT ON SEQUENCES TO %s", si, ident),
		}
		for _, st := range stmts {
			if _, err := conn.Exec(ctx, st); err != nil {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", s, err)
			}
		}
		fmt.Println("  granted on", s)
	}

	// The audit trail is a compliance record. If the application can rewrite it,
	// it is not evidence of anything.
	if _, err := conn.Exec(ctx, fmt.Sprintf("REVOKE UPDATE, DELETE ON platform.audit_log FROM %s", ident)); err != nil {
		fmt.Fprintf(os.Stderr, "revoke on audit_log: %v\n", err)
	} else {
		fmt.Println("  audit_log is append-only for", roleName)
	}

	var super bool
	_ = conn.QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = $1`, roleName).Scan(&super)
	fmt.Printf("\n%s rolsuper = %t (must be false, or row-level security is bypassed)\n", roleName, super)
}

// quoteLiteral escapes a string for use as a SQL literal. Passwords cannot be
// passed as parameters in CREATE ROLE.
func quoteLiteral(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}
