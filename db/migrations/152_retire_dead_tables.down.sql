-- Reverses 152_retire_dead_tables.
--
-- Recreates the twenty tables with their columns, types, defaults and
-- primary keys, generated from the live schema before the drop. Foreign
-- keys and secondary indexes are deliberately not restored: every table
-- here was empty, so there is nothing for them to protect, and their
-- referenced tables have moved on.

BEGIN;

CREATE TABLE IF NOT EXISTS billing.payment_histories (
    id bigserial NOT NULL,
    organization_id bigint NOT NULL,
    payment_id bigint,
    invoice_id bigint,
    action character varying(64) NOT NULL,
    amount numeric(15,2) DEFAULT 0.00 NOT NULL,
    status character varying(32) NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS billing.plan_types (
    id bigserial NOT NULL,
    name character varying(255) NOT NULL,
    code character varying(64) NOT NULL,
    description text,
    sort_order integer DEFAULT 0 NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS billing.user_plan_histories (
    id bigserial NOT NULL,
    user_id bigint NOT NULL,
    organization_id bigint,
    plan_id bigint NOT NULL,
    start_date timestamp with time zone DEFAULT now() NOT NULL,
    end_date timestamp with time zone,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS identity.kyc_records (
    user_id bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    national_id text,
    passport_number text,
    business_license text,
    tax_number text,
    company_name jsonb,
    business_description jsonb,
    industry text,
    identity_verified_at timestamp with time zone,
    notes text,
    reviewed_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS identity.session_plan_requests (
    id bigserial NOT NULL,
    user_id bigint NOT NULL,
    organization_id bigint NOT NULL,
    requested_plan_id bigint,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    admin_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS identity.user_identities (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    provider text NOT NULL,
    provider_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS identity.user_session_histories (
    id bigserial NOT NULL,
    user_id bigint NOT NULL,
    session_id bigint,
    device_uuid character varying(64) NOT NULL,
    ip_address character varying(45) DEFAULT ''::character varying NOT NULL,
    action character varying(64) NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS ingest.import_batches (
    id bigserial NOT NULL,
    public_id character varying(32) DEFAULT ('bat_'::text || replace((gen_random_uuid())::text, '-'::text, ''::text)) NOT NULL,
    session_id bigint NOT NULL,
    organization_id bigint NOT NULL,
    batch_number integer NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    total_rows integer DEFAULT 0 NOT NULL,
    processed_rows integer DEFAULT 0 NOT NULL,
    matched_rows integer DEFAULT 0 NOT NULL,
    error_rows integer DEFAULT 0 NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS inventory.plan_temparte_warehouses (
    id bigserial NOT NULL,
    name jsonb NOT NULL,
    description text,
    price numeric(12,2) DEFAULT 0.00 NOT NULL,
    duration_days integer DEFAULT 30 NOT NULL,
    max_warehouses integer DEFAULT 5 NOT NULL,
    max_rows integer DEFAULT 10000 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS inventory.supplier_trackings (
    id bigserial NOT NULL,
    organization_id bigint NOT NULL,
    supplier_org_id bigint NOT NULL,
    reliability_score numeric(5,2) DEFAULT 100.00 NOT NULL,
    fulfillment_rate numeric(5,2) DEFAULT 100.00 NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS inventory.user_plan_temparte_warehouses (
    id bigserial NOT NULL,
    user_id bigint NOT NULL,
    organization_id bigint,
    plan_id bigint NOT NULL,
    starts_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS notifications.admin_notifications (
    id bigserial NOT NULL,
    event_type character varying(64) NOT NULL,
    title jsonb NOT NULL,
    body jsonb NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_read boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS org.user_organization_numbers (
    id bigint NOT NULL,
    organization_id bigint NOT NULL,
    user_id bigint NOT NULL,
    phone_number text NOT NULL,
    is_verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS platform_admin.ai_providers (
    id bigserial NOT NULL,
    provider_name character varying(64) NOT NULL,
    model_name character varying(128) NOT NULL,
    capability character varying(64) DEFAULT 'product.match'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_working boolean DEFAULT true NOT NULL,
    last_error text,
    context_length integer DEFAULT 4096 NOT NULL,
    price_per_1k numeric(10,6) DEFAULT 0.000000 NOT NULL,
    base_url character varying(255) DEFAULT ''::character varying NOT NULL,
    config_key character varying(128) DEFAULT ''::character varying NOT NULL,
    config_value character varying(255) DEFAULT ''::character varying NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS platform_admin.api_integrations (
    id bigserial NOT NULL,
    organization_id bigint NOT NULL,
    provider character varying(64) NOT NULL,
    credentials jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS platform_admin.system_resources (
    id bigserial NOT NULL,
    key character varying(128) NOT NULL,
    name jsonb NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS profile.user_profiles (
    user_id bigint NOT NULL,
    first_name text,
    last_name text,
    avatar_url text,
    date_of_birth date,
    gender text,
    nationality text,
    bio text,
    slug text,
    website text,
    whatsapp text,
    telegram text,
    secondary_email citext,
    secondary_phone text,
    meta_description text,
    meta_keywords text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    national_id text DEFAULT ''::text NOT NULL,
    passport_number text DEFAULT ''::text NOT NULL,
    latitude numeric(10,8),
    longitude numeric(11,8),
    radius_meters integer DEFAULT 10000 NOT NULL,
    PRIMARY KEY (user_id)
);
CREATE TABLE IF NOT EXISTS promo.offer_package_features (
    id bigserial NOT NULL,
    public_id character varying(32) DEFAULT ('opf_'::text || replace((gen_random_uuid())::text, '-'::text, ''::text)) NOT NULL,
    package_id bigint NOT NULL,
    feature_name jsonb NOT NULL,
    feature_key character varying(64) NOT NULL,
    feature_value character varying(128) DEFAULT ''::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS promo.offer_promotions (
    id bigserial NOT NULL,
    public_id character varying(32) DEFAULT ('opm_'::text || replace((gen_random_uuid())::text, '-'::text, ''::text)) NOT NULL,
    organization_id bigint NOT NULL,
    offer_id bigint NOT NULL,
    plan_id bigint,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    starts_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS promo.offer_views (
    id bigint NOT NULL,
    offer_id bigint NOT NULL,
    user_id bigint,
    organization_id bigint,
    ip_address text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id)
);

COMMIT;
