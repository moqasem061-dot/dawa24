BEGIN;
-- Per-call detail for the integration activity screen (2026-11-10). Every new
-- column has a default, so an older image that inserts without them keeps
-- working. Nothing here holds the key secret, the Authorization header or a
-- request body.
ALTER TABLE erp.api_calls
  ADD COLUMN query        TEXT  NOT NULL DEFAULT '',   -- raw query string, first 500 characters
  ADD COLUMN user_agent   TEXT  NOT NULL DEFAULT '',   -- first 200 characters
  ADD COLUMN bytes_out    INT   NOT NULL DEFAULT 0,    -- response body size
  ADD COLUMN ref          TEXT  NOT NULL DEFAULT '',   -- sync id, order number or external reference the call touched
  ADD COLUMN auth_reason  TEXT  NOT NULL DEFAULT '',   -- why a key with a known prefix was refused
  ADD COLUMN error_detail JSONB;                       -- the error details object, at most 2 KB
CREATE INDEX api_calls_org_id_idx ON erp.api_calls (organization_id, id DESC);
COMMENT ON COLUMN erp.api_calls.query IS 'سلسلة الاستعلام في الرابط، أول 500 حرف';
COMMENT ON COLUMN erp.api_calls.user_agent IS 'برنامج المرسل، أول 200 حرف';
COMMENT ON COLUMN erp.api_calls.bytes_out IS 'حجم الاستجابة بالبايت';
COMMENT ON COLUMN erp.api_calls.ref IS 'المرجع الذي تناوله الطلب: رقم المزامنة أو الطلب أو المرجع الخارجي';
COMMENT ON COLUMN erp.api_calls.auth_reason IS 'سبب رفض المفتاح، لا يظهر للمرسل';
COMMENT ON COLUMN erp.api_calls.error_detail IS 'تفاصيل الخطأ كما أُرسلت للمرسل، بحد 2 كيلوبايت';
COMMIT;
