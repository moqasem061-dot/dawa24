-- 206_seed_notification_templates.down.sql
BEGIN;

DELETE FROM notifications.templates WHERE slug IN (
    'order.placed', 'order.status_changed', 'order.cancelled_by_buyer',
    'purchase_request.created', 'purchase_request.responded',
    'quote.requested', 'quote.provided', 'quote.decision',
    'negotiation.offer', 'negotiation.decision',
    'special_offer.status', 'sponsorship.status', 'ad.status',
    'smart_order.run_finished', 'smart_order.run_failed',
    'import.run_finished', 'import.run_failed',
    'quota.exhausted', 'quota.released',
    'org.profile_change.requested', 'org.profile_change.approved', 'org.profile_change.rejected',
    'org.deletion.requested', 'org.deletion.approved', 'org.deletion.rejected',
    'account.deletion.requested', 'account.deletion.approved', 'account.deletion.rejected',
    'org.approved', 'org.rejected', 'org.suspended', 'org.reactivated',
    'document.requested', 'document.verified', 'document.rejected',
    'branch.created', 'branch.disabled', 'branch.institutional_works.changed',
    'org.user.added', 'org.user.removed', 'user.role.changed', 'user.password.set_by_admin',
    'account.registered', 'admin.new_registration',
    'subscription.updated', 'subscription.expiring', 'subscription.expired',
    'billing.refund.issued',
    'wallet.deposit.pending', 'wallet.deposit.approved', 'wallet.deposit.rejected',
    'wallet.withdrawal.pending', 'wallet.withdrawal.approved', 'wallet.withdrawal.rejected',
    'org.review.created', 'chat.message.offline', 'hr.job_application.received'
);

COMMIT;
