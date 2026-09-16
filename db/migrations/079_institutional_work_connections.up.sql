-- Migration 079: Institutional Work Connections (الاتصالات المسموح بها للكيانات والأنشطة)
BEGIN;

CREATE TABLE IF NOT EXISTS org.institutional_work_connections (
    id                          BIGSERIAL PRIMARY KEY,
    from_institutional_work_id  BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
    to_institutional_work_id    BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_inst_work_conn UNIQUE (from_institutional_work_id, to_institutional_work_id)
);

CREATE INDEX IF NOT EXISTS idx_inst_work_conn_from ON org.institutional_work_connections (from_institutional_work_id);
CREATE INDEX IF NOT EXISTS idx_inst_work_conn_to ON org.institutional_work_connections (to_institutional_work_id);

COMMIT;
