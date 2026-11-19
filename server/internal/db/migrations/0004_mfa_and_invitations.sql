-- v0.4.0: TOTP MFA and one-time invitations / password resets.

ALTER TABLE operator_users ADD COLUMN IF NOT EXISTS mfa_secret TEXT;
ALTER TABLE operator_users ADD COLUMN IF NOT EXISTS mfa_pending_secret TEXT;
ALTER TABLE operator_users ADD COLUMN IF NOT EXISTS mfa_recovery_hashes TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE operator_users ADD COLUMN IF NOT EXISTS mfa_pending_recovery_hashes TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE operator_users ADD COLUMN IF NOT EXISTS mfa_enabled_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS operator_invitations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES operator_users(id),
    membership_id TEXT NOT NULL REFERENCES tenant_memberships(id),
    kind TEXT NOT NULL CHECK (kind IN ('INVITE', 'PASSWORD_RESET')),
    role TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    created_by TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS operator_invitations_tenant_created_idx
    ON operator_invitations (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS operator_invitations_open_user_idx
    ON operator_invitations (user_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS operator_invitations_expiry_idx
    ON operator_invitations (expires_at)
    WHERE used_at IS NULL AND revoked_at IS NULL;
