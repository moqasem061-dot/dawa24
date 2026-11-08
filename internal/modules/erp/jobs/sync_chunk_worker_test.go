package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// The erp package's test fakes live in its own test binary, invisible here, so
// these tests embed the erp.Repository interface (nil everywhere the worker
// path never goes) and stub exactly the calls ProcessChunk and Cleanup make.
// A batch whose items are all terminal exercises the full worker path —
// LoadPendingItems empty, FinishChunk folding, no ports needed.

type stubBatch struct {
	org, id    int64
	state      string
	chunksDone int
	chunkCount int
	acceptedAt time.Time
	finishedAt *time.Time
	startedAt  *time.Time
	publicID   string
	foldCount  int
}

type workerRepo struct {
	erp.Repository
	batches       map[int64]*stubBatch
	earlierActive bool
	purgerCalls   int
	lastCutoffs   erp.Cutoffs
	gotTenant     int64
}

func newWorkerRepo() *workerRepo {
	return &workerRepo{batches: map[int64]*stubBatch{}}
}

func (r *workerRepo) seed(org int64, state string) int64 {
	id := int64(len(r.batches) + 1)
	now := time.Now().UTC()
	r.batches[id] = &stubBatch{org: org, id: id, state: state, chunkCount: 1, acceptedAt: now, publicID: "w-seed"}
	return id
}

func (r *workerRepo) GetBatchByID(_ context.Context, orgID, batchID int64) (erp.SyncBatch, error) {
	r.gotTenant = orgID
	b, ok := r.batches[batchID]
	if !ok || b.org != orgID {
		return erp.SyncBatch{}, erp.ErrNotFound
	}
	return erp.SyncBatch{
		ID: b.id, OrganizationID: b.org, PublicID: b.publicID, IntegrationID: 3,
		WarehouseID: 40, BranchID: 41, State: b.state, ChunkCount: b.chunkCount,
		ChunksDone: b.chunksDone, AcceptedAt: b.acceptedAt, Provider: "orgasoft",
	}, nil
}

func (r *workerRepo) EarlierBatchActive(_ context.Context, orgID, integrationID, batchID int64) (bool, error) {
	return r.earlierActive, nil
}

func (r *workerRepo) MarkBatchProcessing(_ context.Context, orgID, batchID int64) error {
	b, ok := r.batches[batchID]
	if !ok || b.org != orgID {
		return erp.ErrNotFound
	}
	if b.state == "queued" {
		now := time.Now().UTC()
		b.state = "processing"
		b.startedAt = &now
	}
	return nil
}

func (r *workerRepo) LoadPendingItems(context.Context, int64, int64, int) ([]erp.SyncItemRow, error) {
	return nil, nil
}

func (r *workerRepo) FinishChunk(_ context.Context, orgID, batchID int64) (bool, error) {
	b, ok := r.batches[batchID]
	if !ok || b.org != orgID {
		return false, erp.ErrNotFound
	}
	if b.state != "queued" && b.state != "processing" {
		return true, nil
	}
	b.chunksDone++
	folded := b.chunksDone >= b.chunkCount
	if folded {
		// No items on the stub: the fold is a no-op; the batch completes.
		b.state = "completed"
		now := time.Now().UTC()
		b.finishedAt = &now
		b.foldCount++
	}
	return folded, nil
}

func (r *workerRepo) WriteResults(context.Context, int64, []erp.ItemResult, []erp.LinkWrite, []erp.ReviewWrite) error {
	return nil
}

func (r *workerRepo) PurgeExpiredAsSystem(_ context.Context, cut erp.Cutoffs) error {
	r.purgerCalls++
	r.lastCutoffs = cut
	return nil
}

func workerFixture(t *testing.T) (*erp.Service, *workerRepo) {
	t.Helper()
	repo := newWorkerRepo()
	svc := erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: []byte("worker-pepper-32-bytes-long!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: "orgasoft"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, repo
}

func TestSyncChunkWorkerSetsTenant(t *testing.T) {
	svc, repo := workerFixture(t)
	batchID := repo.seed(7, "queued")
	worker := NewSyncChunkWorker(svc, nil)
	job := &river.Job[queue.ERPSyncChunkArgs]{Args: queue.ERPSyncChunkArgs{BatchID: batchID, OrganizationID: 7, ChunkNo: 0}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if repo.gotTenant != 7 {
		t.Errorf("tenant from ctx = %d, want 7", repo.gotTenant)
	}
	if repo.batches[batchID].state != "completed" {
		t.Errorf("state = %q, want completed", repo.batches[batchID].state)
	}
}

func TestSyncChunkWorkerSnoozesOnWaitForEarlier(t *testing.T) {
	svc, repo := workerFixture(t)
	batchID := repo.seed(7, "queued")
	repo.earlierActive = true
	worker := NewSyncChunkWorker(svc, nil)
	job := &river.Job[queue.ERPSyncChunkArgs]{Args: queue.ERPSyncChunkArgs{BatchID: batchID, OrganizationID: 7, ChunkNo: 0}}
	err := worker.Work(context.Background(), job)
	if err == nil {
		t.Fatal("expected the snooze signal")
	}
	var snooze *rivertype.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("err = %v, want rivertype.JobSnoozeError", err)
	}
	if snooze.Duration != erp.SyncSnoozeSeconds*time.Second {
		t.Errorf("snooze = %v, want %d s", snooze.Duration, erp.SyncSnoozeSeconds)
	}
	if repo.batches[batchID].state != "queued" {
		t.Errorf("state = %q, want still queued after snooze", repo.batches[batchID].state)
	}
}

func TestSyncChunkWorkerPassesOtherErrors(t *testing.T) {
	svc, _ := workerFixture(t)
	worker := NewSyncChunkWorker(svc, nil)
	job := &river.Job[queue.ERPSyncChunkArgs]{Args: queue.ERPSyncChunkArgs{BatchID: 99, OrganizationID: 7, ChunkNo: 0}}
	err := worker.Work(context.Background(), job)
	if err == nil {
		t.Fatal("expected the not-found error to pass through")
	}
	var snooze *rivertype.JobSnoozeError
	if errors.As(err, &snooze) {
		t.Fatalf("err = %v, want a pass-through error, not a snooze", err)
	}
}

func TestCleanupWorkerCallsCleanup(t *testing.T) {
	svc, repo := workerFixture(t)
	worker := NewCleanupWorker(svc, nil)
	if err := worker.Work(context.Background(), &river.Job[queue.ERPCleanupArgs]{}); err != nil {
		t.Fatal(err)
	}
	if repo.purgerCalls != 1 {
		t.Errorf("purger calls = %d, want 1", repo.purgerCalls)
	}
	if repo.lastCutoffs.BatchRows != 10000 {
		t.Errorf("cleanup ran with BatchRows = %d", repo.lastCutoffs.BatchRows)
	}
}

var _ = database.ErrNoTenant
