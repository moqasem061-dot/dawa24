package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Per-request database accounting.
//
// A page that is slow under load is slow for one of two reasons the request log
// could not tell apart: it makes many round trips, or it waits for a pool
// connection because every connection is busy. The tracer below counts both
// for any context carrying a QueryStats, and httpx.Logger writes them on every
// request line, so a load test names the pages to fix and which of the two
// problems each has.

// QueryStats accumulates one request's database work. Safe for concurrent use:
// a page may run queries from several goroutines.
type QueryStats struct {
	roundTrips atomic.Int64
	queryNanos atomic.Int64
	waitNanos  atomic.Int64

	mu     sync.Mutex
	shapes map[string]int
}

// maxShapes bounds the per-request statement tally.
const maxShapes = 64

// record tallies a statement by its leading text, which is enough to tell one
// repeated query from another without keeping every statement.
func (s *QueryStats) record(sql string) {
	shape := strings.Join(strings.Fields(sql), " ")
	if len(shape) > 90 {
		shape = shape[:90]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shapes == nil {
		s.shapes = make(map[string]int, 8)
	}
	if _, ok := s.shapes[shape]; ok || len(s.shapes) < maxShapes {
		s.shapes[shape]++
	}
}

// Repeated returns the statements run more than once, most repeated first, at
// most n of them: the signature of a query issued once per row.
func (s *QueryStats) Repeated(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	type kv struct {
		shape string
		count int
	}
	var list []kv
	for k, v := range s.shapes {
		if v > 1 {
			list = append(list, kv{k, v})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].count > list[j].count })
	out := make([]string, 0, n)
	for i := 0; i < len(list) && i < n; i++ {
		out = append(out, fmt.Sprintf("%dx %s", list[i].count, list[i].shape))
	}
	return out
}

// RoundTrips is every statement sent, BEGIN and COMMIT included.
func (s *QueryStats) RoundTrips() int64 { return s.roundTrips.Load() }

// QueryTime is the total time spent in statements.
func (s *QueryStats) QueryTime() time.Duration { return time.Duration(s.queryNanos.Load()) }

// PoolWait is the total time spent waiting for a pool connection.
func (s *QueryStats) PoolWait() time.Duration { return time.Duration(s.waitNanos.Load()) }

type statsKey struct{}

// WithQueryStats attaches a fresh accumulator to ctx.
func WithQueryStats(ctx context.Context) (context.Context, *QueryStats) {
	s := &QueryStats{}
	return context.WithValue(ctx, statsKey{}, s), s
}

func statsFrom(ctx context.Context) *QueryStats {
	s, _ := ctx.Value(statsKey{}).(*QueryStats)
	return s
}

type traceStartKey struct{}

// statsTracer implements pgx.QueryTracer and pgxpool.AcquireTracer.
type statsTracer struct{}

var (
	_ pgx.QueryTracer        = statsTracer{}
	_ pgxpool.AcquireTracer = statsTracer{}
)

func (statsTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s := statsFrom(ctx)
	if s == nil {
		return ctx
	}
	s.record(data.SQL)
	return context.WithValue(ctx, traceStartKey{}, time.Now())
}

func (statsTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	s := statsFrom(ctx)
	start, ok := ctx.Value(traceStartKey{}).(time.Time)
	if s == nil || !ok {
		return
	}
	s.roundTrips.Add(1)
	s.queryNanos.Add(int64(time.Since(start)))
}

func (statsTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	if statsFrom(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, traceStartKey{}, time.Now())
}

func (statsTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	s := statsFrom(ctx)
	start, ok := ctx.Value(traceStartKey{}).(time.Time)
	if s == nil || !ok {
		return
	}
	s.waitNanos.Add(int64(time.Since(start)))
}
