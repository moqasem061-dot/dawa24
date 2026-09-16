package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// cols prints the column list of the named tables, straight from the live
// database. Grepping migrations for this is unreliable: a -A window after one
// CREATE TABLE bleeds into the next.
func cols(dsn string, tables []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	for _, t := range tables {
		parts := strings.SplitN(t, ".", 2)
		if len(parts) != 2 {
			continue
		}
		rows, err := conn.Query(ctx, `
			SELECT column_name FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2
			ORDER BY ordinal_position`, parts[0], parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", t, err)
			continue
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err == nil {
				names = append(names, n)
			}
		}
		rows.Close()
		fmt.Printf("%-36s %s\n", t, strings.Join(names, ", "))
	}
}
