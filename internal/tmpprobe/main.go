package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	c, err := pgx.Connect(ctx, os.Getenv("DSN"))
	if err != nil {
		panic(err)
	}
	defer c.Close(ctx)
	tx, err := c.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		panic(err)
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, "SET LOCAL statement_timeout = '60s'")
	for _, q := range strings.Split(os.Getenv("Q"), ";;") {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		fmt.Println("==", q[:min(len(q), 140)])
		rows, err := tx.Query(ctx, q)
		if err != nil {
			fmt.Println("ERR", err)
			return
		}
		cols := rows.FieldDescriptions()
		var names []string
		for _, f := range cols {
			names = append(names, f.Name)
		}
		fmt.Println(strings.Join(names, " | "))
		for rows.Next() {
			vals, _ := rows.Values()
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = fmt.Sprint(v)
			}
			fmt.Println(strings.Join(parts, " | "))
		}
		rows.Close()
		if rows.Err() != nil {
			fmt.Println("ERR", rows.Err())
			return
		}
	}
}
