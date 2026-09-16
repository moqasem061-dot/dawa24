package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	assistantHTTP "github.com/muhiya/dawa24-store/internal/modules/assistant/http"
	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

// Assembling Capsule.
//
// The pieces are deliberately separate objects: a repository that can only
// write the assistant's own tables, a read model that can only read, a tool
// registry that authorizes, and a service that runs the loop. Nothing in that
// chain can be handed a wider capability by accident, because none of them
// holds one.

type assistantDeps struct {
	db        *database.DB
	cfg       *config.Config
	log       *slog.Logger
	ai        gateway.Client
	cacheH    *cache.Cache
	storage   *storage.Client
	admin     *platformadmin.Service
	adminKeys *adminKeyProvisioner
	keys      assistant.KeyResolver
	// coverage answers the assistant's "who delivers to my branch" tool with
	// the platform's own coverage rule. Left nil, that tool reports itself
	// unavailable rather than guessing.
	coverage assistant.CoverageProbe
	// bridge, when set, is given this assistant so Telegram answers with it.
	bridge *capsuleBridge
	// actions is the dashboard's own code, bound once the UI handler exists.
	actions *lateActions
}

// mountAssistant wires and registers the Capsule assistant.
func mountAssistant(r chi.Router, d assistantDeps) {
	repo := assistantPostgres.NewRepository(d.db)

	// Handles are signed with a key derived from the application secret, so a
	// reference the model produces cannot be forged and one issued to another
	// tenant cannot be replayed. An empty secret (development) yields a random
	// per-process key: handles then stop working across a restart, which
	// nothing depends on, and are never forgeable.
	signer := handles.NewSigner(d.cfg.Session.Secret)

	// The registry takes the repository twice, in two different roles: as the
	// read model (assistant.Reader) and as the audit sink. They are the same
	// object because they are the same database, but the interfaces are
	// separate so a tool can only reach the read half.
	registry := tools.NewRegistry(repo, signer, repo, d.log)
	registry.SetMemoryStore(repo)
	if d.coverage != nil {
		registry.SetCoverageProbe(d.coverage)
	}
	// The governed dataset engine: declared tables only, compiled with the
	// caller's own tenant predicate, executed as the read-only dataset role.
	registry.SetDatasets(datasets.Default(), repo, repo)

	// Proposals, never direct actions: the flow stores what the model asked
	// for, and only a person's confirmation runs it through the dashboard.
	var flow *actions.Flow
	if d.actions != nil {
		flow = actions.NewFlow(repo, d.actions, assistant.CanAct, d.log)
		registry.SetActions(flow, d.actions)
	}

	svc := assistant.NewService(repo, d.ai, registry, d.log)
	svc.SetStorage(d.storage)
	svc.SetKeyResolver(d.keys)
	svc.SetActionFlow(flow)
	svc.SetRefIssuer(registry.BranchRef)

	handler := assistantHTTP.NewHandler(svc, repo, d.ai, assistantBuffer(d), d.log)
	handler.SetStorage(d.storage)
	handler.SetExports(repo)
	handler.SetKeyResolver(d.keys)
	handler.SetTranscriptionModelResolver(transcriptionResolver(d))
	handler.RegisterRoutes(r)

	// Telegram gets this exact service and this exact rate limiter.
	d.bridge.bind(svc, repo, handler.AllowQuestion)
}

// assistantBuffer picks where in-flight answers live.
//
// Redis in production, so a turn survives a reconnect that lands on a different
// replica and a rolling deploy of the one that started it. Process memory in
// development, where there is no Redis and one replica — same interface, same
// behaviour, no configuration to get wrong.
func assistantBuffer(d assistantDeps) stream.Buffer {
	// Resolved on first use, not here.
	//
	// Routes are mounted while the Redis connection is still being dialled in
	// the background, so asking now always answered "no Redis" and production
	// silently spent its whole life on the in-process buffer — the one that
	// cannot serve a reader reconnecting to another replica.
	return stream.NewLazyBuffer(func() *redis.Client {
		if d.cacheH == nil {
			return nil
		}
		return d.cacheH.Redis()
	})
}

// transcriptionResolver discovers which model can turn speech into text.
//
// It has to ask the Gateway's ADMIN catalogue rather than /v1/models, because
// /v1/models excludes transcription models by design and the one the Gateway
// ships is seeded inactive. Hardcoding a name is how the microphone button came
// to answer 404 on a fresh deployment.
func transcriptionResolver(d assistantDeps) gateway.TranscriptionModelResolver {
	cacheEntry := gateway.NewTranscriptionModelCache(
		func(ctx context.Context) ([]gateway.GatewayModel, error) {
			client, err := adminClientFor(ctx, d.admin)
			if err != nil {
				return nil, err
			}
			return client.ListModels(ctx)
		},
		func(context.Context) string {
			// An operator may pin a model with the same environment variable
			// that overrides every other model role. Empty means "choose the
			// cheapest active one that accepts this audio".
			return os.Getenv("GATEWAY_MODEL_ASSISTANT_TRANSCRIBE")
		},
	)
	return cacheEntry.Resolve
}

// adminClientFor builds a Gateway management client from saved settings.
func adminClientFor(ctx context.Context, admin *platformadmin.Service) (*gateway.AdminClient, error) {
	if admin == nil {
		return nil, errGatewayNotConfigured
	}
	gw, err := admin.GetGatewaySettings(database.AsSystem(ctx))
	if err != nil {
		return nil, err
	}
	if gw == nil || !gw.IsActive || gw.EndpointURL == "" || !gw.CanProvision() {
		return nil, errGatewayNotConfigured
	}
	username, password := gw.AdminCredentials()
	return gateway.NewAdminClient(gw.EndpointURL, username, password), nil
}
