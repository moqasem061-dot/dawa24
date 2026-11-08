BEGIN;
CREATE SEQUENCE IF NOT EXISTS commerce.order_number_seq;
SELECT setval('commerce.order_number_seq',
  GREATEST((SELECT COALESCE(MAX(order_number::bigint), 0) FROM commerce.orders
            WHERE order_number ~ '^[0-9]{1,8}$'), 0) + 1000, false);
COMMENT ON SEQUENCE commerce.order_number_seq IS 'متسلسلة أرقام الطلبات: تضمن عدم تكرار الرقم بين النسخ المتعددة للخادم';
DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='dawa24_app') THEN
  GRANT USAGE, SELECT ON SEQUENCE commerce.order_number_seq TO dawa24_app;
END IF; END $$;
COMMIT;
