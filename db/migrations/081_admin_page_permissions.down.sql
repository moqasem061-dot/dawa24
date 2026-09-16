BEGIN;

DELETE FROM identity.permissions
WHERE key IN (
  'catalog.product.view', 'catalog.product.update', 'catalog.product.delete',
  'catalog.category.view', 'catalog.category.update', 'catalog.category.delete',
  'catalog.brand.view', 'catalog.brand.update', 'catalog.brand.delete',
  'catalog.saving_product.view', 'catalog.saving_product.update',
  'org.organization.view', 'org.organization.update', 'org.organization.delete',
  'org.branch.view', 'org.branch.update', 'org.branch.delete',
  'org.institutional_work.view', 'org.institutional_work.update', 'org.institutional_work.delete',
  'org.role.view', 'org.role.update', 'org.role.delete',
  'org.review.view', 'org.review.update', 'org.review.delete',
  'identity.user.view', 'identity.user.update', 'identity.user.delete',
  'identity.admin_role.view', 'identity.admin_role.update', 'identity.admin_role.delete',
  'commerce.order.view', 'commerce.order.update', 'commerce.quote.view',
  'billing.invoice.view', 'billing.payment.view', 'billing.payment.update',
  'billing.subscription_plan.view', 'billing.subscription_plan.update',
  'billing.session_plan.view', 'billing.session_plan.update',
  'promo.offer.view', 'promo.offer.update',
  'promo.ad.view', 'promo.ad.update', 'promo.ad.delete',
  'promo.ad_plan.view', 'promo.ad_plan.update',
  'inventory.warehouse.view', 'inventory.warehouse.update', 'inventory.warehouse.delete',
  'inventory.stock.view', 'inventory.transfer.view', 'inventory.transfer.update',
  'ingest.session.view', 'ingest.session.update',
  'hr.job.view', 'hr.job.update', 'hr.job.delete',
  'hr.document.view', 'hr.document.update',
  'workflow.issue.view', 'workflow.issue.update',
  'workflow.request.view', 'workflow.request.update',
  'platform.setting.view', 'platform.setting.update',
  'platform.content.view', 'platform.content.update', 'platform.content.delete',
  'platform.activity_log.view', 'platform.activity_log.delete',
  'platform.error_log.view', 'platform.error_log.update', 'platform.error_log.delete',
  'platform.developer.sql',
  'platform.trash.view', 'platform.trash.update'
);

COMMIT;
