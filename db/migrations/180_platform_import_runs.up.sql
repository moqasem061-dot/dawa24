-- 180_platform_import_runs.up.sql
--
-- Durable import session table. Replaces the six in-memory session stores
-- (saving products, team, compare, temp warehouse, vendor ingest, admin
-- catalogue) with a single, restartable, observable, queue-backed table.
--
-- Staged rows live in the child table import_run_rows rather than in
-- payload JSONB, so a 30k-row file does not bloat the parent row.

BEGIN;

-- ──────────────────────────────────────────────────────────────────────
-- Parent: one row per import attempt.
-- ──────────────────────────────────────────────────────────────────────
CREATE TABLE platform.import_runs (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id       UUID   NOT NULL DEFAULT gen_random_uuid(),
  organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
  user_id         BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,

  -- What kind of import is this?
  kind            TEXT NOT NULL
                    CHECK (kind IN (
                      'saving_products', 'team', 'compare',
                      'temp_warehouse', 'catalog', 'catalog_images'
                    )),
  audience        TEXT NOT NULL DEFAULT ''
                    CHECK (audience IN ('', 'vendor', 'customer', 'admin')),

  filename        TEXT NOT NULL DEFAULT '',

  -- Lifecycle state machine.
  state           TEXT NOT NULL DEFAULT 'queued'
                    CHECK (state IN (
                      'queued', 'processing', 'ready',
                      'committing', 'committed', 'failed', 'cancelled'
                    )),

  -- Human-readable phase label (e.g. "loading catalogue", "matching row 42 of 9000").
  phase           TEXT NOT NULL DEFAULT '',

  -- 0..100 progress for the frontend bar.
  percent         SMALLINT NOT NULL DEFAULT 0
                    CHECK (percent BETWEEN 0 AND 100),

  -- Row counts.
  total_rows      INTEGER NOT NULL DEFAULT 0,
  processed_rows  INTEGER NOT NULL DEFAULT 0,

  -- Mapping choices, column indices, flags — everything the worker needs
  -- that is not a staged row.  Kept small on purpose; rows go in the
  -- child table.
  payload         JSONB  NOT NULL DEFAULT '{}',

  -- Summary counters written by the worker when state reaches 'ready'.
  result          JSONB  NOT NULL DEFAULT '{}',

  -- Filled on state = 'failed'.
  error_message   TEXT   NOT NULL DEFAULT '',

  -- If the work was enqueued via River, record the job id so we can
  -- cancel it.
  river_job_id    BIGINT,

  started_at      TIMESTAMPTZ,
  finished_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Uniqueness on the public-facing id.
CREATE UNIQUE INDEX idx_import_runs_public_id
  ON platform.import_runs (public_id);

-- The list-sessions query: "my org's recent imports".
CREATE INDEX idx_import_runs_org_created
  ON platform.import_runs (organization_id, created_at DESC);

-- The sweeper query: "anything stuck in processing/committing".
CREATE INDEX idx_import_runs_active_state
  ON platform.import_runs (state)
  WHERE state IN ('queued', 'processing', 'committing');

-- Trigger: keep updated_at fresh.
CREATE TRIGGER trg_import_runs_updated_at
  BEFORE UPDATE ON platform.import_runs
  FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();


-- ──────────────────────────────────────────────────────────────────────
-- Child: one row per data row in the imported file.
--
-- The data column is JSONB so every import kind can store its own shape
-- (StagedSavingItem, TeamImportRow, compare row, catalogue staging row)
-- without a migration per field.
-- ──────────────────────────────────────────────────────────────────────
CREATE TABLE platform.import_run_rows (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id          BIGINT NOT NULL REFERENCES platform.import_runs(id) ON DELETE CASCADE,
  row_number      INTEGER NOT NULL,

  -- The full parsed row, import-kind-specific.
  data            JSONB  NOT NULL DEFAULT '{}',

  -- Whether the user chose to include this row in the final commit.
  included        BOOLEAN NOT NULL DEFAULT TRUE,

  -- Optional: matched catalogue product for saving/ingest imports.
  matched_product_id BIGINT,

  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup: "all rows for this run, in order".
CREATE INDEX idx_import_run_rows_run_num
  ON platform.import_run_rows (run_id, row_number);

-- Idempotent commit: ON CONFLICT (run_id, row_number) DO NOTHING.
CREATE UNIQUE INDEX idx_import_run_rows_run_row_unique
  ON platform.import_run_rows (run_id, row_number);

-- Trigger: keep updated_at fresh.
CREATE TRIGGER trg_import_run_rows_updated_at
  BEFORE UPDATE ON platform.import_run_rows
  FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();

COMMIT;
