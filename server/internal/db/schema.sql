CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE products ADD COLUMN IF NOT EXISTS detail TEXT NOT NULL DEFAULT '';

-- The MVP originally created ACTIVE/ARCHIVED only. Keep startup migration
-- idempotent while extending existing deployments with the draft workflow.
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_status_check;
ALTER TABLE products
    ADD CONSTRAINT products_status_check
    CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED'));

CREATE TABLE IF NOT EXISTS assets (
    id TEXT PRIMARY KEY,
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
    product_id TEXT NOT NULL REFERENCES products(id),
    object_key TEXT NOT NULL UNIQUE,
    alt_text TEXT NOT NULL DEFAULT '',
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redeem_codes (
    id TEXT PRIMARY KEY,
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
    entitlement_id TEXT NOT NULL REFERENCES entitlements(id),
    asset_id TEXT NOT NULL REFERENCES assets(id),
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS download_events (
    id TEXT PRIMARY KEY,
    entitlement_id TEXT NOT NULL REFERENCES entitlements(id),
    asset_id TEXT NOT NULL REFERENCES assets(id),
    result TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS admin_events (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL DEFAULT 'token-admin',
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS join_applications (
    id TEXT PRIMARY KEY,
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

CREATE TABLE IF NOT EXISTS platform_settings (
    id TEXT PRIMARY KEY CHECK (id = 'default'),
    deployment_mode TEXT NOT NULL DEFAULT 'SPLIT_ORIGIN' CHECK (deployment_mode IN ('COLOCATED', 'SPLIT_ORIGIN', 'SPLIT_APPS')),
    storefront_url TEXT NOT NULL DEFAULT 'http://localhost:45173',
    console_url TEXT NOT NULL DEFAULT 'http://localhost:45173',
    api_base_url TEXT NOT NULL DEFAULT 'http://localhost:48080',
    operator_web_path TEXT NOT NULL DEFAULT '/ops-7f3c',
    operator_api_path TEXT NOT NULL DEFAULT 'cp-7f3c',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO platform_settings (id)
VALUES ('default')
ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS redeem_codes_product_status_idx ON redeem_codes(product_id, status);
CREATE INDEX IF NOT EXISTS product_images_product_created_idx ON product_images(product_id, created_at);
CREATE INDEX IF NOT EXISTS entitlements_product_status_idx ON entitlements(product_id, status);
CREATE INDEX IF NOT EXISTS download_tickets_expiry_idx ON download_tickets(expires_at);
CREATE INDEX IF NOT EXISTS download_events_entitlement_idx ON download_events(entitlement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_events_resource_idx ON admin_events(resource_type, resource_id, created_at DESC);

-- Public demo catalog. These rows contain descriptions only; no sellable files or credentials.
INSERT INTO products (id, name, description, detail, status)
VALUES
    ('demo-nover-starter', 'NOVER Starter Kit', '知屿品牌起步模板，包含界面结构、主题 token 与交付页示例。', '适合快速搭建数字产品交付首页与兑换入口的合成示例。', 'ACTIVE'),
    ('demo-react-interface', 'React Interface Pack', '适合产品原型的 React 页面组件练习包，使用合成示例数据。', '包含一组适合产品原型的界面结构练习，重点展示响应式布局与状态反馈。', 'ACTIVE'),
    ('demo-go-api-blueprint', 'Go API Blueprint', '面向本地开发的 Go API 结构示例，帮助快速了解 NOVER 的交付流程。', '从健康检查、商品目录到兑换与下载票据，展示一个小型 Go 服务的组织方式。', 'ACTIVE')
ON CONFLICT (id) DO UPDATE SET detail = EXCLUDED.detail WHERE products.detail = '';
