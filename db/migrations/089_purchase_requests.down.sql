-- 089_purchase_requests.down.sql

BEGIN;

DROP TABLE IF EXISTS commerce.purchase_request_lines;
DROP TABLE IF EXISTS commerce.purchase_requests;

COMMIT;
