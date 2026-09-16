package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/safe"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

// Assistant retention.
//
// Conversations are deleted six months after they were created. That is a
// promise the drawer makes to the user in words, so it is kept by a job rather
// than by anyone remembering: the deadline is a column with an index on it, the
// sweep is a DELETE against that index, and messages, turns and attachment rows
// go with it by cascade.
//
// Two sweeps run here because they have different clocks. Conversations expire
// on a six-month schedule; uploads that were never actually sent with a
// question are worthless after a day and cost object storage until they go.
func startAssistantRetention(
	ctx context.Context, db *database.DB, storageCfg config.Storage, log *slog.Logger,
) {
	// The sweep owns its storage client rather than taking one from main:
	// object storage being unavailable must not stop the database half of the
	// retention promise, which is the half that actually deletes the data.
	var store *storage.Client
	if sc, err := storage.New(ctx, storageCfg); err == nil {
		store = sc
	} else {
		log.Warn("assistant retention: object storage unavailable; "+
			"attachment rows will still be deleted, objects will not", "error", err)
	}

	repo := assistantPostgres.NewRepository(db)
	svc := assistant.NewService(repo, nil, nil, log)

	sweep := func(trigger string) {
		// Cross-tenant by nature: it sweeps every organisation's history.
		sysCtx := database.AsSystem(ctx)

		if n, err := svc.PurgeExpiredConversations(sysCtx); err != nil {
			log.Error("assistant conversation retention failed", "trigger", trigger, "error", err)
		} else if n > 0 {
			log.Info("assistant conversations purged",
				"trigger", trigger, "count", n, "retention_months", 6)
		}

		if n, err := repo.PurgeExpiredExports(sysCtx, time.Now()); err != nil {
			log.Error("assistant export sweep failed", "trigger", trigger, "error", err)
		} else if n > 0 {
			log.Info("assistant exports purged", "trigger", trigger, "count", n)
		}

		keys, err := svc.PurgeOrphanAttachments(sysCtx)
		if err != nil {
			log.Error("assistant attachment sweep failed", "trigger", trigger, "error", err)
			return
		}
		if len(keys) == 0 {
			return
		}
		// The rows are already gone. Objects are deleted best-effort: a failure
		// here leaves a file nobody can reach through the application, which is
		// a storage cost rather than an exposure.
		deleted := 0
		for _, key := range keys {
			if store == nil {
				break
			}
			if err := store.Delete(sysCtx, key); err != nil {
				log.Warn("assistant attachment object not deleted", "key", key, "error", err)
				continue
			}
			deleted++
		}
		log.Info("assistant orphan attachments purged", "rows", len(keys), "objects", deleted)
	}

	safe.Go(log, "worker-assistant-retention", func() {
		// A short delay after boot rather than immediately: the pool is still
		// warming and nothing here is urgent.
		select {
		case <-ctx.Done():
			return
		case <-time.After(45 * time.Second):
			sweep("startup")
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep("daily")
			}
		}
	})
}
