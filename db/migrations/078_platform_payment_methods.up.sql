-- Migration 078: Platform Supported Payment Methods & Channels Configuration
CREATE TABLE IF NOT EXISTS billing.platform_payment_methods (
    id VARCHAR(64) PRIMARY KEY,
    name_ar VARCHAR(255) NOT NULL,
    name_en VARCHAR(255) NOT NULL,
    provider_type VARCHAR(64) NOT NULL DEFAULT 'bank', -- 'bank', 'instapay', 'wallet', 'card', 'cash'
    description_ar TEXT NOT NULL DEFAULT '',
    description_en TEXT NOT NULL DEFAULT '',
    
    -- Structured Banking / Payout Credentials
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    bank_name VARCHAR(255) NOT NULL DEFAULT '',
    account_number VARCHAR(128) NOT NULL DEFAULT '',
    iban VARCHAR(128) NOT NULL DEFAULT '',
    swift_code VARCHAR(64) NOT NULL DEFAULT '',
    branch_name VARCHAR(255) NOT NULL DEFAULT '',
    
    -- InstaPay / E-Wallet Credentials
    instapay_handle VARCHAR(128) NOT NULL DEFAULT '',
    phone_number VARCHAR(64) NOT NULL DEFAULT '',
    
    -- Configuration & Status
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_deposit_enabled BOOLEAN NOT NULL DEFAULT true,
    is_checkout_enabled BOOLEAN NOT NULL DEFAULT true,
    display_order INT NOT NULL DEFAULT 0,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed initial default platform channels
INSERT INTO billing.platform_payment_methods (
    id, name_ar, name_en, provider_type, description_ar, description_en,
    account_name, bank_name, account_number, iban, swift_code, branch_name,
    instapay_handle, phone_number, is_active, is_deposit_enabled, is_checkout_enabled, display_order
) VALUES
(
    'instapay',
    'إنستاباي InstaPay (التحويل اللحظي المباشر)',
    'InstaPay Direct Instant Transfer',
    'instapay',
    'تحويل فوري عبر منظومة البنك المركزي المصري وشبكة المدفوعات اللحظية IPN',
    'Instant transfer via Central Bank of Egypt IPN network',
    'شركة دواء 24 للتجارة والتوزيع',
    'البنك المركزي المصري IPN',
    '01065397000',
    '',
    '',
    '',
    'dawa24@instapay',
    '01065397000',
    true, true, true, 1
),
(
    'bank_transfer',
    'تحويل بنكي رسمي (CIB / الأهلي / بنك مصر)',
    'Official Corporate Bank Transfer',
    'bank',
    'إيداع أو تحويل بنكي رسمي إلى حساب الشركة المعتمد في البنوك المصرية',
    'Direct corporate wire transfer to official Egyptian bank accounts',
    'شركة دواء 24 للتجارة والتوزيع ذ.م.م',
    'البنك التجاري الدولي (CIB)',
    '100048291048',
    'EG3800100004829104800000000000',
    'CIBEEGCX',
    'فرع المهندسين الرئيسي',
    '',
    '',
    true, true, true, 2
),
(
    'card',
    'بطاقة دفع إلكتروني (Visa / Mastercard / Meeza)',
    'Credit / Debit Card Online Payment',
    'card',
    'دفع آمن وفوري عبر بطاقات الائتمان وبطاقات ميزة الوطنية',
    'Secure online payment via credit, debit, or Meeza cards',
    'منصة دواء 24 للتجارة الإلكترونية',
    'بوابة الدفع الإلكتروني المعتمدة',
    '',
    '',
    '',
    '',
    '',
    '',
    true, true, true, 3
),
(
    'vodafone_cash',
    'محافظ الهاتف المحمول (فودافون كاش / أورنج / وي / اتصالات)',
    'Mobile Wallets (Vodafone Cash / Orange / WE / e&)',
    'wallet',
    'التحويل المباشر عبر المحافظ الإلكترونية لشبكات المحمول في مصر',
    'Mobile money wallets payment in Egypt',
    'محفظة أعمال دواء 24',
    'فودافون مصر',
    '01065397000',
    '',
    '',
    '',
    '',
    '01065397000',
    true, true, true, 4
)
ON CONFLICT (id) DO UPDATE SET
    name_ar = EXCLUDED.name_ar,
    name_en = EXCLUDED.name_en,
    provider_type = EXCLUDED.provider_type,
    account_name = EXCLUDED.account_name,
    bank_name = EXCLUDED.bank_name,
    account_number = EXCLUDED.account_number,
    iban = EXCLUDED.iban,
    swift_code = EXCLUDED.swift_code,
    branch_name = EXCLUDED.branch_name,
    instapay_handle = EXCLUDED.instapay_handle,
    phone_number = EXCLUDED.phone_number,
    updated_at = now();
