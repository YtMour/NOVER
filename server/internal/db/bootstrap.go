package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultTenantID     = "tenant-default"
	bootstrapUserID     = "user-bootstrap"
	bootstrapMembership = "membership-bootstrap"
)

// EnsureBootstrapIdentity creates the first local platform owner once. Later
// credential changes are handled through the account lifecycle, not startup.
func EnsureBootstrapIdentity(ctx context.Context, pool *pgxpool.Pool, username, password, tenantName string) error {
	username = strings.TrimSpace(username)
	tenantName = strings.TrimSpace(tenantName)
	if username == "" || password == "" || tenantName == "" {
		return errors.New("bootstrap username, password, and tenant name are required")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bootstrap identity: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE tenants SET name=$2, updated_at=NOW() WHERE id=$1`, defaultTenantID, tenantName); err != nil {
		return fmt.Errorf("update default tenant: %w", err)
	}

	var userID string
	err = tx.QueryRow(ctx, `SELECT id FROM operator_users WHERE LOWER(username)=LOWER($1)`, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingUsers int
		if countErr := tx.QueryRow(ctx, `SELECT COUNT(*) FROM operator_users`).Scan(&existingUsers); countErr != nil {
			return fmt.Errorf("count operator users: %w", countErr)
		}
		if existingUsers > 0 {
			return tx.Commit(ctx)
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return fmt.Errorf("hash bootstrap password: %w", hashErr)
		}
		userID = bootstrapUserID
		if _, err := tx.Exec(ctx, `INSERT INTO operator_users (id, username, display_name, password_hash, platform_role) VALUES ($1,$2,$2,$3,'PLATFORM_OWNER')`, userID, username, string(hash)); err != nil {
			return fmt.Errorf("create bootstrap user: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("find bootstrap user: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO tenant_memberships (id, tenant_id, user_id, role) VALUES ($1,$2,$3,'OWNER') ON CONFLICT (tenant_id, user_id) DO NOTHING`, bootstrapMembership, defaultTenantID, userID); err != nil {
		return fmt.Errorf("create bootstrap membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bootstrap identity: %w", err)
	}
	return nil
}
