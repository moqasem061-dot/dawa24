package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// recordingTx records the statements it is asked to run.
type recordingTx struct {
	pgx.Tx
	sql          []string
	savepointErr error
}

func (t *recordingTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	t.sql = append(t.sql, sql)
	if sql == "SAVEPOINT "+softSavepoint {
		return pgconn.CommandTag{}, t.savepointErr
	}
	return pgconn.CommandTag{}, nil
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A failed best-effort statement is rolled back to its savepoint, so the
// transaction around it stays usable — the whole point of Soft.
func TestSoftRollsBackOnlyTheFailedStatement(t *testing.T) {
	tx := &recordingTx{}
	boom := errors.New("column does not exist")
	if err := Soft(context.Background(), tx, func() error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the statement's own error", err)
	}
	want := []string{"SAVEPOINT dawa_soft", "ROLLBACK TO SAVEPOINT dawa_soft", "RELEASE SAVEPOINT dawa_soft"}
	if !equal(tx.sql, want) {
		t.Fatalf("statements = %q, want %q", tx.sql, want)
	}
}

func TestSoftReleasesOnSuccess(t *testing.T) {
	tx := &recordingTx{}
	if err := SoftExec(context.Background(), tx, "UPDATE x SET y = 1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"SAVEPOINT dawa_soft", "UPDATE x SET y = 1", "RELEASE SAVEPOINT dawa_soft"}
	if !equal(tx.sql, want) {
		t.Fatalf("statements = %q, want %q", tx.sql, want)
	}
}

// Outside a transaction block there is nothing to abort; the statement runs.
func TestSoftRunsPlainWhenNoSavepointIsPossible(t *testing.T) {
	tx := &recordingTx{savepointErr: errors.New("SAVEPOINT can only be used in transaction blocks")}
	ran := false
	if err := Soft(context.Background(), tx, func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
}
