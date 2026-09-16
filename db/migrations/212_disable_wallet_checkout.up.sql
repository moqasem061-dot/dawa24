-- Migration 212: Ensure wallet debit is disabled at checkout and ensure COD method exists
INSERT INTO billing.platform_payment_methods (
    id, name_ar, name_en, provider_type, description_ar, description_en,
    is_active, is_deposit_enabled, is_checkout_enabled, display_order
) VALUES (
    'wallet',
    'السحب من المحفظة',
    'Wallet Balance',
    'wallet',
    'الخصم المباشر من رصيد المحفظة لسداد الطلبيات',
    'Direct debit from wallet balance to pay for orders',
    true, false, false, 99
) ON CONFLICT (id) DO UPDATE SET
    is_checkout_enabled = false,
    updated_at = now();

INSERT INTO billing.platform_payment_methods (
    id, name_ar, name_en, provider_type, description_ar, description_en,
    is_active, is_deposit_enabled, is_checkout_enabled, display_order
) VALUES (
    'cod',
    'الدفع نقداً عند الاستلام (COD)',
    'Cash on Delivery',
    'cash',
    'سداد قيمة الطلبية نقداً لمندوب الشحن عند استلام البضاعة',
    'Pay cash to courier upon order delivery',
    true, false, true, 0
) ON CONFLICT (id) DO NOTHING;

-- Explicitly ensure wallet checkout is disabled
UPDATE billing.platform_payment_methods
SET is_checkout_enabled = false
WHERE id = 'wallet';
