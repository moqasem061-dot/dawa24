-- Migration 079 (down): Drop Institutional Work Connections Table
BEGIN;

DROP TABLE IF EXISTS org.institutional_work_connections;

COMMIT;
