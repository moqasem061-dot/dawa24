BEGIN;
CREATE INDEX IF NOT EXISTS idx_user_orgs_vendor_number
  ON org.user_organizations (vendor_org_id, lower(organization_number))
  WHERE deleted_at IS NULL AND organization_number <> '';
COMMENT ON INDEX org.idx_user_orgs_vendor_number IS 'فهرس البحث برقم العميل لدى المورد في ERP؛ غير فريد لأن التكرارات القديمة تبقى كما هي';
COMMIT;
