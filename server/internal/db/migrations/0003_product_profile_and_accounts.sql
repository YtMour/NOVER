-- v0.3.0: product profile v2, update-policy snapshots, audit queries.
-- All statements are additive and safe for existing installations; update policy
-- columns on codes/entitlements stay NULL for legacy rows (treated as LIFETIME).

ALTER TABLE products ADD COLUMN IF NOT EXISTS highlights TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE products ADD COLUMN IF NOT EXISTS license_terms TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS requirements TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS support_period TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS update_policy TEXT NOT NULL DEFAULT 'LIFETIME';
ALTER TABLE products ADD COLUMN IF NOT EXISTS update_until TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'products_update_policy_check') THEN
        ALTER TABLE products
            ADD CONSTRAINT products_update_policy_check
            CHECK (update_policy IN ('LIFETIME', 'TILL_DATE', 'CURRENT_ONLY')) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'products_update_until_check') THEN
        ALTER TABLE products
            ADD CONSTRAINT products_update_until_check
            CHECK ((update_policy = 'TILL_DATE' AND update_until IS NOT NULL)
                OR (update_policy <> 'TILL_DATE' AND update_until IS NULL)) NOT VALID;
    END IF;
END $$;

ALTER TABLE products VALIDATE CONSTRAINT products_update_policy_check;
ALTER TABLE products VALIDATE CONSTRAINT products_update_until_check;

ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS update_policy TEXT;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS update_until TIMESTAMPTZ;
ALTER TABLE entitlements ADD COLUMN IF NOT EXISTS update_policy TEXT;
ALTER TABLE entitlements ADD COLUMN IF NOT EXISTS update_until TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'redeem_codes_update_policy_check') THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_update_policy_check
            CHECK (update_policy IS NULL OR update_policy IN ('LIFETIME', 'TILL_DATE', 'CURRENT_ONLY')) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'entitlements_update_policy_check') THEN
        ALTER TABLE entitlements
            ADD CONSTRAINT entitlements_update_policy_check
            CHECK (update_policy IS NULL OR update_policy IN ('LIFETIME', 'TILL_DATE', 'CURRENT_ONLY')) NOT VALID;
    END IF;
END $$;

ALTER TABLE redeem_codes VALIDATE CONSTRAINT redeem_codes_update_policy_check;
ALTER TABLE entitlements VALIDATE CONSTRAINT entitlements_update_policy_check;

CREATE INDEX IF NOT EXISTS admin_events_tenant_created_idx
    ON admin_events (tenant_id, created_at DESC, id DESC);

-- Give the public demo catalog a small, synthetic profile so the new sections
-- are visible in a fresh install. Existing operator data is not touched.
UPDATE products SET
    highlights = ARRAY['开箱即用的品牌与页面结构', '可复用的交付与兑换流程', '合成示例数据，可直接替换'],
    license_terms = '示例内容，仅用于演示商品资料结构；实际授权以运营者发布为准。',
    requirements = '本地开发需 Node.js 24+ 与 Go 1.24+；无需外部服务。',
    support_period = '示例支持期：30 天',
    update_policy = 'LIFETIME'
WHERE id = 'demo-nover-starter' AND highlights = '{}';

UPDATE products SET
    highlights = ARRAY['React 页面组件练习', '响应式布局与状态反馈', '可独立拆分的界面结构'],
    license_terms = '示例内容，仅用于演示商品资料结构。',
    requirements = 'Node.js 24+；不依赖特定 UI 框架。',
    support_period = '示例支持期：90 天',
    update_policy = 'TILL_DATE',
    update_until = NOW() + INTERVAL '180 days'
WHERE id = 'demo-react-interface' AND highlights = '{}';

UPDATE products SET
    highlights = ARRAY['Go 小型服务结构示例', '健康检查、目录与兑换流程', '便于本地运行的交付蓝图'],
    license_terms = '示例内容，仅用于演示商品资料结构。',
    requirements = 'Go 1.24+、PostgreSQL 17。',
    support_period = '示例支持期：永久（演示）',
    update_policy = 'CURRENT_ONLY'
WHERE id = 'demo-go-api-blueprint' AND highlights = '{}';
