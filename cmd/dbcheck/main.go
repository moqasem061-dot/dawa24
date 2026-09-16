// Command dbcheck inspects a live Dawa24 database.
// and its extensions if asked.
//
// Run with -create to make changes; without it, this only reads.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	create := flag.Bool("create", false, "create the database and extensions")
	doVerify := flag.String("verify", "", "verify schema state at this DSN")
	scopeDSN := flag.String("scoping", "", "report scoping columns of unprotected tables")
	colsDSN := flag.String("cols", "", "print columns of tables given as arguments")
	provisionDSN := flag.String("provision", "", "create the least-privilege app role at this DSN")
	rehashDSN := flag.String("rehash", "", "report applied migrations whose recorded hash differs only by line endings")
	nullscanDSN := flag.String("nullscan", "", "report nullable text columns selected into non-pointer scans")
	auditDSN := flag.String("audit", "", "print the most recent platform.audit_log rows")
	rehashApply := flag.Bool("rehash-apply", false, "write the corrected hashes")
	roleName := flag.String("role", "dawa24_app", "application role name")
	rolePass := flag.String("password", "", "application role password")
	target := flag.String("db", "dawa24-store", "target database name")
	admin := flag.String("admin", "", "admin connection string (to the postgres database)")
	flag.Parse()

	if *auditDSN != "" {
		auditPeek(*auditDSN)
		return
	}

	if *nullscanDSN != "" {
		nullscan(*nullscanDSN)
		return
	}

	if *rehashDSN != "" {
		rehash(*rehashDSN, *rehashApply)
		return
	}

	if *provisionDSN != "" {
		if *rolePass == "" {
			fmt.Fprintln(os.Stderr, "-password is required with -provision")
			os.Exit(1)
		}
		provision(*provisionDSN, *roleName, *rolePass)
		return
	}

	if *colsDSN != "" {
		cols(*colsDSN, flag.Args())
		return
	}

	if *scopeDSN != "" {
		scoping(*scopeDSN)
		return
	}

	if *doVerify != "" {
		verify(*doVerify)
		return
	}

	if *admin == "" {
		fmt.Fprintln(os.Stderr, "-admin is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, *admin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err == nil {
		fmt.Println("server:", strings.SplitN(version, " on ", 2)[0])
	}

	fmt.Println("\nexisting databases:")
	rows, err := conn.Query(ctx,
		`SELECT datname, pg_size_pretty(pg_database_size(datname))
		 FROM pg_database WHERE datistemplate = false ORDER BY datname`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list databases: %v\n", err)
		os.Exit(1)
	}
	found := false
	for rows.Next() {
		var name, size string
		if err := rows.Scan(&name, &size); err != nil {
			break
		}
		marker := " "
		if name == *target {
			marker, found = "*", true
		}
		fmt.Printf("  %s %-24s %s\n", marker, name, size)
	}
	rows.Close()

	if found {
		fmt.Printf("\n%q already exists.\n", *target)
		return
	}
	if !*create {
		fmt.Printf("\n%q does not exist. Re-run with -create to create it.\n", *target)
		return
	}

	// CREATE DATABASE cannot run inside a transaction, hence plain Exec.
	// template0 with an explicit encoding: on a shared instance, template1 may
	// already carry objects from another project.
	stmt := fmt.Sprintf(
		`CREATE DATABASE %s WITH ENCODING 'UTF8' LC_COLLATE 'C.UTF-8' LC_CTYPE 'C.UTF-8' TEMPLATE template0`,
		pgx.Identifier{*target}.Sanitize())
	if _, err := conn.Exec(ctx, stmt); err != nil {
		fmt.Fprintf(os.Stderr, "create database: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\ncreated database %q\n", *target)

	// Extensions need superuser rights the application role will not have, so
	// they are created here. Migration 001's CREATE EXTENSION IF NOT EXISTS then
	// becomes a no-op instead of a permission error that aborts the run.
	targetDSN := strings.Replace(*admin, "/postgres?", "/"+*target+"?", 1)
	tconn, err := pgx.Connect(ctx, targetDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to new database: %v\n", err)
		os.Exit(1)
	}
	defer tconn.Close(ctx)

	for _, ext := range []string{"pg_trgm", "unaccent", "pgcrypto", "citext"} {
		if _, err := tconn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+ext); err != nil {
			fmt.Fprintf(os.Stderr, "create extension %s: %v\n", ext, err)
			os.Exit(1)
		}
		fmt.Println("  extension ready:", ext)
	}
}
