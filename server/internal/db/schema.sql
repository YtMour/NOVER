CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

CREATE INDEX IF NOT EXISTS redeem_codes_product_status_idx ON redeem_codes(product_id, status);
CREATE INDEX IF NOT EXISTS entitlements_product_status_idx ON entitlements(product_id, status);
CREATE INDEX IF NOT EXISTS download_tickets_expiry_idx ON download_tickets(expires_at);
CREATE INDEX IF NOT EXISTS download_events_entitlement_idx ON download_events(entitlement_id, created_at DESC);
CREATE INDEX IF NOT EXISTS admin_events_resource_idx ON admin_events(resource_type, resource_id, created_at DESC);

-- Public demo catalog. These rows contain descriptions only; no sellable files or credentials.
INSERT INTO products (id, name, description, status)
VALUES
    ('demo-nover-starter', 'NOVER Starter Kit', '知屿品牌起步模板，包含界面结构、主题 token 与交付页示例。', 'ACTIVE'),
    ('demo-react-interface', 'React Interface Pack', '适合产品原型的 React 页面组件练习包，使用合成示例数据。', 'ACTIVE'),
    ('demo-go-api-blueprint', 'Go API Blueprint', '面向本地开发的 Go API 结构示例，帮助快速了解 NOVER 的交付流程。', 'ACTIVE')
ON CONFLICT (id) DO NOTHING;
