CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO tenants (id, name, slug)
VALUES ('tenant-default', 'NOVER Default Space', 'default')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS operator_users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    platform_role TEXT CHECK (platform_role IS NULL OR platform_role IN ('PLATFORM_OWNER', 'PLATFORM_SUPPORT', 'PLATFORM_AUDITOR')),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    token_version INTEGER NOT NULL DEFAULT 1 CHECK (token_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS operator_users_username_idx ON operator_users (LOWER(username));

CREATE TABLE IF NOT EXISTS tenant_memberships (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES operator_users(id),
    role TEXT NOT NULL CHECK (role IN ('OWNER', 'ADMIN', 'EDITOR', 'FULFILLMENT', 'ANALYST')),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id)
);

CREATE TABLE IF NOT EXISTS operator_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES operator_users(id),
    membership_id TEXT NOT NULL REFERENCES tenant_memberships(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    token_version INTEGER NOT NULL CHECK (token_version > 0),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE operator_sessions ADD COLUMN IF NOT EXISTS token_version INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS operator_sessions_user_active_idx
    ON operator_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    owner_user_id TEXT REFERENCES operator_users(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE products ADD COLUMN IF NOT EXISTS detail TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS owner_user_id TEXT REFERENCES operator_users(id);

-- The MVP originally created ACTIVE/ARCHIVED only. Keep startup migration
-- idempotent while extending existing deployments with the draft workflow.
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_status_check;
ALTER TABLE products
    ADD CONSTRAINT products_status_check
    CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED'));

ALTER TABLE tenant_memberships DROP CONSTRAINT IF EXISTS tenant_memberships_status_check;
ALTER TABLE tenant_memberships
    ADD CONSTRAINT tenant_memberships_status_check
    CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED'));

CREATE TABLE IF NOT EXISTS assets (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    object_key TEXT NOT NULL UNIQUE,
    file_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS product_images (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    object_key TEXT NOT NULL UNIQUE,
    alt_text TEXT NOT NULL DEFAULT '',
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redeem_codes (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    code_hmac BYTEA NOT NULL UNIQUE,
    display_suffix TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ASSIGNED' CHECK (status IN ('ASSIGNED', 'REDEEMED', 'SUSPENDED', 'REFUNDED', 'REVOKED', 'EXPIRED')),
    external_order_ref TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    access_days INTEGER NOT NULL CHECK (access_days > 0),
    max_downloads INTEGER NOT NULL CHECK (max_downloads > 0),
    redeemed_at TIMESTAMPTZ,
    entitlement_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS entitlements (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    redeem_code_id TEXT NOT NULL UNIQUE REFERENCES redeem_codes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    access_token_hash BYTEA NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED', 'REVOKED', 'EXPIRED')),
    valid_until TIMESTAMPTZ NOT NULL,
    max_downloads INTEGER NOT NULL CHECK (max_downloads > 0),
    downloads_issued INTEGER NOT NULL DEFAULT 0 CHECK (downloads_issued >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS redeem_codes_id_product_id_idx ON redeem_codes(id, product_id);
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'entitlements_redeem_product_fk'
    ) THEN
        ALTER TABLE entitlements
            ADD CONSTRAINT entitlements_redeem_product_fk
            FOREIGN KEY (redeem_code_id, product_id)
            REFERENCES redeem_codes(id, product_id);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'redeem_codes_entitlement_fk'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_entitlement_fk
            FOREIGN KEY (entitlement_id) REFERENCES entitlements(id)
            DEFERRABLE INITIALLY DEFERRED;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS download_tickets (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    entitlement_id TEXT NOT NULL REFERENCES entitlements(id),
    asset_id TEXT NOT NULL REFERENCES assets(id),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS download_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    entitlement_id TEXT NOT NULL REFERENCES entitlements(id),
    asset_id TEXT NOT NULL REFERENCES assets(id),
    result TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS admin_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    actor TEXT NOT NULL DEFAULT 'token-admin',
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS join_applications (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id),
    operator_user_id TEXT REFERENCES operator_users(id),
    organization_name TEXT NOT NULL,
    contact_name TEXT NOT NULL,
    email TEXT NOT NULL,
    desired_username TEXT NOT NULL,
    use_case TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'REVIEWING', 'APPROVED', 'REJECTED')),
    reviewer TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS join_applications_open_email_idx
    ON join_applications (LOWER(email))
    WHERE status IN ('PENDING', 'REVIEWING', 'APPROVED');
CREATE INDEX IF NOT EXISTS join_applications_status_created_idx
    ON join_applications (status, created_at DESC);
ALTER TABLE join_applications ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE join_applications ADD COLUMN IF NOT EXISTS operator_user_id TEXT REFERENCES operator_users(id);
CREATE UNIQUE INDEX IF NOT EXISTS join_applications_open_username_idx
    ON join_applications (LOWER(desired_username))
    WHERE status IN ('PENDING', 'REVIEWING', 'APPROVED');

CREATE TABLE IF NOT EXISTS platform_settings (
    id TEXT PRIMARY KEY CHECK (id = 'default'),
    deployment_mode TEXT NOT NULL DEFAULT 'SPLIT_ORIGIN' CHECK (deployment_mode IN ('COLOCATED', 'SPLIT_ORIGIN', 'SPLIT_APPS')),
    storefront_url TEXT NOT NULL DEFAULT 'http://localhost:45173',
    console_url TEXT NOT NULL DEFAULT 'http://localhost:46173',
    api_base_url TEXT NOT NULL DEFAULT 'http://localhost:48080',
    operator_web_path TEXT NOT NULL DEFAULT '/ops-7f3c',
    operator_api_path TEXT NOT NULL DEFAULT 'cp-7f3c',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO platform_settings (id)
VALUES ('default')
ON CONFLICT (id) DO NOTHING;

UPDATE platform_settings
SET deployment_mode='SPLIT_APPS', console_url='http://localhost:46173', updated_at=NOW()
WHERE id='default' AND deployment_mode='SPLIT_ORIGIN'
  AND storefront_url='http://localhost:45173' AND console_url='http://localhost:45173';

CREATE INDEX IF NOT EXISTS redeem_codes_product_status_idx ON redeem_codes(product_id, status);
ALTER TABLE assets ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE product_images ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE entitlements ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE download_tickets ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE download_events ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
ALTER TABLE admin_events ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'tenant-default' REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS products_tenant_status_created_idx ON products(tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS products_tenant_owner_idx ON products(tenant_id, owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS assets_tenant_product_idx ON assets(tenant_id, product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS product_images_tenant_product_idx ON product_images(tenant_id, product_id, created_at);
CREATE INDEX IF NOT EXISTS redeem_codes_tenant_product_status_idx ON redeem_codes(tenant_id, product_id, status);
CREATE INDEX IF NOT EXISTS entitlements_tenant_product_status_idx ON entitlements(tenant_id, product_id, status);
CREATE INDEX IF NOT EXISTS download_tickets_tenant_expiry_idx ON download_tickets(tenant_id, expires_at);
CREATE INDEX IF NOT EXISTS download_events_tenant_created_idx ON download_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_events_tenant_resource_idx ON admin_events(tenant_id, resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS product_images_product_created_idx ON product_images(product_id, created_at);
CREATE INDEX IF NOT EXISTS entitlements_product_status_idx ON entitlements(product_id, status);
CREATE INDEX IF NOT EXISTS download_tickets_expiry_idx ON download_tickets(expires_at);
CREATE INDEX IF NOT EXISTS download_events_entitlement_idx ON download_events(entitlement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_events_resource_idx ON admin_events(resource_type, resource_id, created_at DESC);

-- Tenant-scoped uniqueness and foreign keys make the isolation boundary a
-- database invariant instead of relying only on every application query.
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_id_idx ON products(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS assets_tenant_id_idx ON assets(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS product_images_tenant_id_idx ON product_images(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS redeem_codes_tenant_product_id_idx ON redeem_codes(tenant_id, id, product_id);
CREATE UNIQUE INDEX IF NOT EXISTS entitlements_tenant_id_idx ON entitlements(tenant_id, id);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'products_owner_tenant_fk') THEN
        ALTER TABLE products
            ADD CONSTRAINT products_owner_tenant_fk
            FOREIGN KEY (tenant_id, owner_user_id)
            REFERENCES tenant_memberships(tenant_id, user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assets_product_tenant_fk') THEN
        ALTER TABLE assets
            ADD CONSTRAINT assets_product_tenant_fk
            FOREIGN KEY (tenant_id, product_id)
            REFERENCES products(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'product_images_product_tenant_fk') THEN
        ALTER TABLE product_images
            ADD CONSTRAINT product_images_product_tenant_fk
            FOREIGN KEY (tenant_id, product_id)
            REFERENCES products(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'redeem_codes_product_tenant_fk') THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_product_tenant_fk
            FOREIGN KEY (tenant_id, product_id)
            REFERENCES products(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'entitlements_product_tenant_fk') THEN
        ALTER TABLE entitlements
            ADD CONSTRAINT entitlements_product_tenant_fk
            FOREIGN KEY (tenant_id, product_id)
            REFERENCES products(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'entitlements_redeem_tenant_fk') THEN
        ALTER TABLE entitlements
            ADD CONSTRAINT entitlements_redeem_tenant_fk
            FOREIGN KEY (tenant_id, redeem_code_id, product_id)
            REFERENCES redeem_codes(tenant_id, id, product_id)
            DEFERRABLE INITIALLY DEFERRED;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'redeem_codes_entitlement_tenant_fk') THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_entitlement_tenant_fk
            FOREIGN KEY (tenant_id, entitlement_id)
            REFERENCES entitlements(tenant_id, id)
            DEFERRABLE INITIALLY DEFERRED;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'download_tickets_entitlement_tenant_fk') THEN
        ALTER TABLE download_tickets
            ADD CONSTRAINT download_tickets_entitlement_tenant_fk
            FOREIGN KEY (tenant_id, entitlement_id)
            REFERENCES entitlements(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'download_tickets_asset_tenant_fk') THEN
        ALTER TABLE download_tickets
            ADD CONSTRAINT download_tickets_asset_tenant_fk
            FOREIGN KEY (tenant_id, asset_id)
            REFERENCES assets(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'download_events_entitlement_tenant_fk') THEN
        ALTER TABLE download_events
            ADD CONSTRAINT download_events_entitlement_tenant_fk
            FOREIGN KEY (tenant_id, entitlement_id)
            REFERENCES entitlements(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'download_events_asset_tenant_fk') THEN
        ALTER TABLE download_events
            ADD CONSTRAINT download_events_asset_tenant_fk
            FOREIGN KEY (tenant_id, asset_id)
            REFERENCES assets(tenant_id, id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'join_applications_operator_tenant_fk') THEN
        ALTER TABLE join_applications
            ADD CONSTRAINT join_applications_operator_tenant_fk
            FOREIGN KEY (tenant_id, operator_user_id)
            REFERENCES tenant_memberships(tenant_id, user_id);
    END IF;
END $$;

-- New rows are checked immediately; validating here also protects existing
-- installations during the idempotent startup migration.
ALTER TABLE products VALIDATE CONSTRAINT products_owner_tenant_fk;
ALTER TABLE assets VALIDATE CONSTRAINT assets_product_tenant_fk;
ALTER TABLE product_images VALIDATE CONSTRAINT product_images_product_tenant_fk;
ALTER TABLE redeem_codes VALIDATE CONSTRAINT redeem_codes_product_tenant_fk;
ALTER TABLE entitlements VALIDATE CONSTRAINT entitlements_product_tenant_fk;
ALTER TABLE entitlements VALIDATE CONSTRAINT entitlements_redeem_tenant_fk;
ALTER TABLE redeem_codes VALIDATE CONSTRAINT redeem_codes_entitlement_tenant_fk;
ALTER TABLE download_tickets VALIDATE CONSTRAINT download_tickets_entitlement_tenant_fk;
ALTER TABLE download_tickets VALIDATE CONSTRAINT download_tickets_asset_tenant_fk;
ALTER TABLE download_events VALIDATE CONSTRAINT download_events_entitlement_tenant_fk;
ALTER TABLE download_events VALIDATE CONSTRAINT download_events_asset_tenant_fk;
ALTER TABLE join_applications VALIDATE CONSTRAINT join_applications_operator_tenant_fk;

-- Public demo catalog. These rows contain descriptions only; no sellable files or credentials.
INSERT INTO products (id, name, description, detail, status)
VALUES
    ('demo-nover-starter', 'NOVER Starter Kit', '知屿品牌起步模板，包含界面结构、主题 token 与交付页示例。', '适合快速搭建数字产品交付首页与兑换入口的合成示例。', 'ACTIVE'),
    ('demo-react-interface', 'React Interface Pack', '适合产品原型的 React 页面组件练习包，使用合成示例数据。', '包含一组适合产品原型的界面结构练习，重点展示响应式布局与状态反馈。', 'ACTIVE'),
    ('demo-go-api-blueprint', 'Go API Blueprint', '面向本地开发的 Go API 结构示例，帮助快速了解 NOVER 的交付流程。', '从健康检查、商品目录到兑换与下载票据，展示一个小型 Go 服务的组织方式。', 'ACTIVE')
ON CONFLICT (id) DO UPDATE SET detail = EXCLUDED.detail WHERE products.detail = '';
