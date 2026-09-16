// Package config loads and validates all runtime configuration from the
// environment, once, at boot.
//
// The rule this package enforces: the process refuses to start rather than run
// misconfigured. The legacy system had CACHE_STORE=database and CACHE_DRIVER=redis
// set simultaneously, with the second key silently ignored by the framework — so
// caching ran on MySQL for months without anyone noticing. Unknown or missing
// configuration must be loud.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	EnvDev     Env = "dev"
	EnvStaging Env = "staging"
	EnvProd    Env = "prod"
)

func (e Env) IsProd() bool { return e == EnvProd }

type Config struct {
	Env       Env
	AppName   string
	BaseURL   string
	HTTP      HTTP
	Database  Database
	Redis     Redis
	Storage   Storage
	Gateway   Gateway
	Maps      Maps
	Session   Session
	Observ    Observability
	Worker    Worker
	Scrape    AntiScrape
	Telegram  Telegram
	WhatsApp  WhatsApp
	Marketing Marketing
}

// AntiScrape governs the guard on the signed-out pages that publish
// marketplace data. See internal/platform/antiscrape.
type AntiScrape struct {
	// Enabled turns the guard into a passthrough when false. Kept switchable
	// because the failure mode of a mistuned budget is refusing customers, and
	// an operator needs to be able to stop that without a deploy.
	Enabled bool
	// PenaltyTTL is how long a honeypot trip keeps a caller refused.
	PenaltyTTL time.Duration
	// GuestMaxPage and GuestMaxPageSize bound how far into a listing a caller
	// who has not signed in may page. This is the ceiling on how much of the
	// catalogue one anonymous session can collect at all; the budgets only
	// govern how fast.
	GuestMaxPage     int
	GuestMaxPageSize int
}

type Database struct {
	URL              string
	MaxConns         int32
	MinConns         int32
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	StatementTimeout time.Duration
	// WorkerStatementTimeout is the same ceiling for the background worker,
	// which needs its own: statement_timeout is a RuntimeParam on every pooled
	// connection, so the web's thirty seconds also capped every statement
	// inside a background import — however long River's JobTimeout was.
	WorkerStatementTimeout time.Duration
}

type Redis struct {
	URL      string
	PoolSize int
}

type Storage struct {
	Endpoint        string // MinIO or S3 endpoint
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool // MinIO needs this; AWS S3 does not
	PublicBaseURL   string
}

// Gateway is the connection to the MuhiyaLLM Gateway.
//
// This struct is the ONLY place in the Store that knows an AI service exists at
// all. There is deliberately no provider name, no model id, and no provider API
// key anywhere in this configuration: model selection and provider routing are
// the Gateway's responsibility, addressed through capability aliases.
type Gateway struct {
	BaseURL    string        // https://api.muhiya.com
	VirtualKey string        // issued by the Gateway admin, per environment
	ClientApp  string        // populates request_logs.client_app on the Gateway
	Timeout    time.Duration // per-request ceiling; capabilities may lower it
	Enabled    bool          // false => every capability serves its fallback
}

// Maps is the Google Maps Embed API configuration. The key is optional: without
// it, MapPicker renders a coordinate-entry fallback instead of an embedded map.
// The key ships to browsers inside iframe URLs by design, so it MUST be
// restricted by HTTP referrer in Google Cloud Console — an unrestricted key is
// a billing incident waiting to happen.
type Maps struct {
	GoogleMapsAPIKey string
}

type Session struct {
	CookieName  string
	Secret      string
	TTL         time.Duration
	IdleTimeout time.Duration
	SecureOnly  bool
}

type Observability struct {
	LogLevel      string
	LogFormat     string // "json" or "text"
	OTLPEndpoint  string
	TraceSampling float64
	MetricsPort   int
}

type Worker struct {
	Queues          map[string]int // queue name -> worker count
	ShutdownTimeout time.Duration
}

// Load reads the environment and returns a validated Config for the server and
// worker, or an aggregate of everything that is wrong. Reporting all problems at
// once beats making an operator restart six times to discover six missing
// variables.
func Load() (*Config, error) { return load(false) }

// LoadForCLI is Load with the requirements the CLI genuinely has.
//
// Migrations and other operational commands touch PostgreSQL and nothing else.
// Demanding REDIS_URL and a 32-byte SESSION_SECRET from them meant an operator
// had to invent two values the command never reads before it would run — which
// is enough friction that migrations get skipped, and skipped migrations are how
// a deploy serves traffic against a schema it does not expect.
func LoadForCLI() (*Config, error) { return load(true) }

func load(cliOnly bool) (*Config, error) {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	env := Env(getStr("APP_ENV", string(EnvDev)))
	switch env {
	case EnvDev, EnvStaging, EnvProd:
	default:
		fail("APP_ENV must be one of dev|staging|prod, got %q", env)
	}

	cfg := &Config{
		Env:     env,
		AppName: getStr("APP_NAME", "Dawa24"),
		BaseURL: getStr("APP_BASE_URL", "http://localhost:8080"),

		HTTP: loadHTTP(env),

		Scrape: AntiScrape{
			Enabled:          getBool("ANTISCRAPE_ENABLED", true),
			PenaltyTTL:       getDuration("ANTISCRAPE_PENALTY_TTL", time.Hour),
			GuestMaxPage:     getInt("ANTISCRAPE_GUEST_MAX_PAGE", 25),
			GuestMaxPageSize: getInt("ANTISCRAPE_GUEST_MAX_PAGE_SIZE", 48),
		},

		Database: Database{
			URL:              getStr("DATABASE_URL", ""),
			MaxConns:         int32(getInt("DB_MAX_CONNS", 8)),
			MinConns:         int32(getInt("DB_MIN_CONNS", 1)),
			MaxConnLifetime:  getDuration("DB_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime:  getDuration("DB_MAX_CONN_IDLE", 30*time.Minute),
			StatementTimeout: getDuration("DB_STATEMENT_TIMEOUT", 30*time.Second),
			// Long enough for the largest real import statement, short enough
			// that a runaway background query still lets go of its connection.
			WorkerStatementTimeout: getDuration("DB_WORKER_STATEMENT_TIMEOUT", 10*time.Minute),
		},

		Redis: Redis{
			URL:      getStr("REDIS_URL", ""),
			PoolSize: getInt("REDIS_POOL_SIZE", 10),
		},

		Storage: Storage{
			Endpoint:        getStr("STORAGE_ENDPOINT", ""),
			Region:          getStr("STORAGE_REGION", "us-east-1"),
			Bucket:          getStr("STORAGE_BUCKET", "dawa24"),
			AccessKeyID:     getStr("STORAGE_ACCESS_KEY_ID", ""),
			SecretAccessKey: getStr("STORAGE_SECRET_ACCESS_KEY", ""),
			UsePathStyle:    getBool("STORAGE_USE_PATH_STYLE", true),
			PublicBaseURL:   getStr("STORAGE_PUBLIC_BASE_URL", ""),
		},

		Gateway: Gateway{
			BaseURL:    getStr("GATEWAY_BASE_URL", "https://api.muhiya.com"),
			VirtualKey: getStr("GATEWAY_VIRTUAL_KEY", ""),
			ClientApp:  getStr("GATEWAY_CLIENT_APP", "dawa24-store"),
			Timeout:    getDuration("GATEWAY_TIMEOUT", 60*time.Second),
			Enabled:    getBool("GATEWAY_ENABLED", false),
		},

		Maps: Maps{
			GoogleMapsAPIKey: getStr("GOOGLE_MAPS_API_KEY", ""),
		},

		Session: Session{
			CookieName:  getStr("SESSION_COOKIE_NAME", "dawa24_session"),
			Secret:      getStr("SESSION_SECRET", ""),
			TTL:         getDuration("SESSION_TTL", 720*time.Hour), // 30 days, matching legacy
			IdleTimeout: getDuration("SESSION_IDLE_TIMEOUT", 30*time.Minute),
			SecureOnly:  getBool("SESSION_SECURE", env != EnvDev),
		},

		Observ: Observability{
			LogLevel:      getStr("LOG_LEVEL", "info"),
			LogFormat:     getStr("LOG_FORMAT", "json"),
			OTLPEndpoint:  getStr("OTLP_ENDPOINT", ""),
			TraceSampling: getFloat("TRACE_SAMPLING", 0.05),
			MetricsPort:   getInt("METRICS_PORT", 9090),
		},

		Worker: Worker{
			Queues: map[string]int{
				// Separate pools so that a vendor uploading 500k SKUs cannot
				// starve order confirmations or notification delivery.
				// Sized for 2-core VPS (8 total workers instead of 15).
				"imports":       getInt("WORKER_IMPORTS", 1),
				"ai":            getInt("WORKER_AI", 2),
				"notifications": getInt("WORKER_NOTIFICATIONS", 2),
				"projections":   getInt("WORKER_PROJECTIONS", 1),
				"maintenance":   getInt("WORKER_MAINTENANCE", 1),
				// River polls ONLY the queues named here, and jobs.go inserts
				// SmartOrderRunArgs into "smartorder" — absent from this map, a
				// job there sat in river_job for ever while its registered
				// worker idled. Guarded by TestEveryInsertedQueueIsConfigured.
				"smartorder": getInt("WORKER_SMARTORDER", 1),
			},
			ShutdownTimeout: getDuration("WORKER_SHUTDOWN_TIMEOUT", 60*time.Second),
		},
	}

	cfg.Telegram = loadTelegram(fail)
	cfg.WhatsApp = loadWhatsApp(fail)
	cfg.Marketing = loadMarketing(fail)

	// --- Required everywhere ---
	if cfg.Database.URL == "" {
		fail("DATABASE_URL is required")
	} else if _, err := url.Parse(cfg.Database.URL); err != nil {
		fail("DATABASE_URL is not a valid URL: %v", err)
	}
	if !cliOnly {
		if cfg.Redis.URL == "" {
			fail("REDIS_URL is required (cache, sessions and rate limiting all depend on it)")
		}
		if len(cfg.Session.Secret) < 32 {
			fail("SESSION_SECRET is required and must be at least 32 bytes")
		}
	}

	// --- Gateway coherence ---
	// Enabled without a key is the failure mode that would otherwise surface as
	// a 401 from the Gateway on the first vendor import, hours after deploy.
	if cfg.Gateway.Enabled && cfg.Gateway.VirtualKey == "" {
		fail("GATEWAY_ENABLED=true requires GATEWAY_VIRTUAL_KEY")
	}
	if cfg.Gateway.BaseURL != "" {
		if u, err := url.Parse(cfg.Gateway.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			fail("GATEWAY_BASE_URL must be an absolute URL, got %q", cfg.Gateway.BaseURL)
		} else if env.IsProd() && u.Scheme != "https" {
			fail("GATEWAY_BASE_URL must use https in production")
		}
	}

	validateHTTP(cfg, fail)

	// --- Production-only strictness ---
	if env.IsProd() && !cliOnly {
		if cfg.Storage.Endpoint != "" && (cfg.Storage.AccessKeyID == "" || cfg.Storage.SecretAccessKey == "") {
			fail("STORAGE_ACCESS_KEY_ID and STORAGE_SECRET_ACCESS_KEY are required when STORAGE_ENDPOINT is configured")
		}
		if !cfg.Session.SecureOnly {
			fail("SESSION_SECURE must not be disabled in production")
		}
		if strings.HasPrefix(cfg.BaseURL, "http://") &&
			!strings.HasPrefix(cfg.BaseURL, "http://localhost") &&
			!strings.HasPrefix(cfg.BaseURL, "http://127.0.0.1") {
			fail("APP_BASE_URL must use https in production")
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// --- typed getters ---

func getStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func getCSV(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ErrNoConfig is returned by helpers that require a loaded config.
var ErrNoConfig = errors.New("config: not loaded")

// DatabasePassword extracts the password from a PostgreSQL DSN.
//
// It exists so the Gateway settings screen can be told the one secret it must
// never accept: the password this process connects to its own database with.
// A malformed or password-less DSN yields "", which disables that check rather
// than blocking configuration.
func DatabasePassword(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.User == nil {
		return ""
	}
	pass, ok := u.User.Password()
	if !ok {
		return ""
	}
	return pass
}
