-- v0.2.0: indexes used by the maintenance command and expiry lookups.
-- Keep this file immutable once released; add 0003_... for further changes.
CREATE INDEX IF NOT EXISTS operator_sessions_expires_idx
    ON operator_sessions (expires_at);

CREATE INDEX IF NOT EXISTS operator_sessions_revoked_idx
    ON operator_sessions (revoked_at)
    WHERE revoked_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS redeem_codes_assigned_expiry_idx
    ON redeem_codes (expires_at)
    WHERE status = 'ASSIGNED';

CREATE INDEX IF NOT EXISTS entitlements_active_valid_until_idx
    ON entitlements (valid_until)
    WHERE status = 'ACTIVE';
