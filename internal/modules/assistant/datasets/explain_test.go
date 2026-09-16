package datasets

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestEveryPlanIsValidSQL asks PostgreSQL to plan every compiled statement
// against a real schema. EXPLAIN plans without executing, so it is safe against
// any database, and it is the only check that a declaration's column names and
// joins match the migrations rather than the author's memory.
//
// Set ASSISTANT_DATASETS_DSN to run it.
func TestEveryPlanIsValidSQL(t *testing.T) {
	dsn := os.Getenv("ASSISTANT_DATASETS_DSN")
	if dsn == "" {
		t.Skip("ASSISTANT_DATASETS_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	cat := Default()
	for _, d := range cat.All() {
		name := d.Name
		a := owner(d.Scope, 1, 1)
		reqs := []Request{{Dataset: name}, {Dataset: name, Metrics: []string{"count"}}}
		if hasSearch(d) {
			reqs = append(reqs, Request{Dataset: name, Search: "x"})
		}
		for _, f := range d.Fields {
			switch f.Type {
			case Text, Enum, Bool, Int:
				reqs = append(reqs, Request{Dataset: name, GroupBy: []string{f.Name}, Metrics: []string{"count"}})
			case Date, Time:
				reqs = append(reqs, Request{Dataset: name, GroupBy: []string{f.Name + ":week"}, Metrics: []string{"min:" + f.Name}})
			case Money, Number:
				reqs = append(reqs, Request{Dataset: name, Metrics: []string{"sum:" + f.Name, "max:" + f.Name}})
			}
		}
		if d.Key != nil {
			p, err := Compile(a, d, Request{Dataset: name}, Options{MaxLimit: 1, KeyID: 1})
			if err != nil {
				t.Fatalf("%s key: %v", name, err)
			}
			explain(ctx, t, conn, name+" key", p)
		}
		for kind := range d.Parents {
			p, err := Compile(a, d, Request{Dataset: name}, Options{MaxLimit: 10, ParentKind: kind, ParentID: 1})
			if err != nil {
				t.Fatalf("%s parent %s: %v", name, kind, err)
			}
			explain(ctx, t, conn, name+" parent", p)
		}
		for _, req := range reqs {
			p, err := Compile(a, d, req, rowOpts)
			if err != nil {
				t.Fatalf("%s %+v: %v", name, req, err)
			}
			explain(ctx, t, conn, name, p)
		}
	}
}

func hasSearch(d *Dataset) bool {
	for _, f := range d.Fields {
		if f.Search {
			return true
		}
	}
	return false
}

// explain plans a statement. With ASSISTANT_DATASETS_ROLE set it plans it as
// that role, which is how a declaration reading an ungranted table or column
// is caught: PostgreSQL checks privileges when it plans.
func explain(ctx context.Context, t *testing.T, conn *pgx.Conn, label string, p *Plan) {
	t.Helper()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if role := os.Getenv("ASSISTANT_DATASETS_ROLE"); role != "" {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatalf("set role: %v", err)
		}
	}
	rows, err := tx.Query(ctx, "EXPLAIN "+p.SQL, p.Args...)
	if err == nil {
		rows.Close()
		err = rows.Err()
	}
	if err != nil {
		t.Errorf("%s: %v\n%s", label, err, p.SQL)
	}
}
