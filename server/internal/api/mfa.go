package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/YtMour/NOVER/server/internal/totp"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const mfaChallengeTTL = 5 * time.Minute

type mfaChallengeClaims struct {
	Version   int    `json:"v"`
	UserID    string `json:"uid"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
	Purpose   string `json:"purpose"`
}

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func (a *App) mfaStatus(w http.ResponseWriter, r *http.Request) {
	identity, _ := operatorIdentityFromContext(r.Context())
	var enabledAt *time.Time
	var recoveryCount *int
	err := a.db.QueryRow(r.Context(), `SELECT mfa_enabled_at, array_length(mfa_recovery_hashes, 1) FROM operator_users WHERE id=$1`, identity.UserID).Scan(&enabledAt, &recoveryCount)
	if err != nil {
		a.logError(r, "load mfa status failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA status could not be loaded.")
		return
	}
	remaining := 0
	if recoveryCount != nil {
		remaining = *recoveryCount
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":                  enabledAt != nil,
		"enabled_at":               enabledAt,
		"recovery_codes_remaining": remaining,
	})
}

func (a *App) mfaSetup(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "mfa-setup", 10, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many MFA setup attempts. Please wait and try again.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	var enabled bool
	if err := a.db.QueryRow(r.Context(), `SELECT mfa_enabled_at IS NOT NULL FROM operator_users WHERE id=$1`, identity.UserID).Scan(&enabled); err != nil {
		a.logError(r, "check mfa enabled failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be configured.")
		return
	}
	if enabled {
		writeError(w, http.StatusConflict, "mfa_already_enabled", "Two-factor authentication is already enabled.")
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		a.logError(r, "generate mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be configured.")
		return
	}
	encrypted, err := a.encryptMFASecret(secret)
	if err != nil {
		a.logError(r, "encrypt mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be configured.")
		return
	}
	recoveryCodes := make([]string, 0, 10)
	recoveryHashes := make([]string, 0, 10)
	for index := 0; index < 10; index++ {
		code, err := newRecoveryCode()
		if err != nil {
			a.logError(r, "generate recovery code failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be configured.")
			return
		}
		recoveryCodes = append(recoveryCodes, code)
		recoveryHashes = append(recoveryHashes, hashRecoveryCode(code))
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE operator_users SET mfa_pending_secret=$2, mfa_pending_recovery_hashes=$3 WHERE id=$1`, identity.UserID, encrypted, recoveryHashes); err != nil {
		a.logError(r, "store pending mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be configured.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":         secret,
		"otpauth_url":    totp.URI("NOVER", identity.Username, secret),
		"recovery_codes": recoveryCodes,
	})
}

func (a *App) mfaEnable(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter the authenticator code.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	var pendingSecret string
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(mfa_pending_secret, '') FROM operator_users WHERE id=$1`, identity.UserID).Scan(&pendingSecret); err != nil {
		a.logError(r, "load pending mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be enabled.")
		return
	}
	if pendingSecret == "" {
		writeError(w, http.StatusConflict, "mfa_setup_required", "Start MFA setup before enabling it.")
		return
	}
	secret, err := a.decryptMFASecret(pendingSecret)
	if err != nil || !totp.Validate(secret, input.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid_mfa_code", "The authenticator code is not valid. Check your device clock and try again.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin mfa enable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be enabled.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE operator_users SET mfa_secret=mfa_pending_secret, mfa_recovery_hashes=mfa_pending_recovery_hashes, mfa_enabled_at=NOW(), mfa_pending_secret=NULL, mfa_pending_recovery_hashes='{}' WHERE id=$1`, identity.UserID); err != nil {
		a.logError(r, "enable mfa failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be enabled.")
		return
	}
	if err := recordOperatorAudit(r, tx, identity.TenantID, "MFA_ENABLED", identity.UserID, map[string]string{"username": identity.Username}); err != nil {
		a.logError(r, "record mfa enable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be enabled.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit mfa enable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be enabled.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (a *App) mfaDisable(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "mfa-disable", 10, time.Hour); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many MFA attempts. Please wait and try again.")
		return
	}
	var input struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter your password and authenticator code.")
		return
	}
	identity, _ := operatorIdentityFromContext(r.Context())
	var passwordHash, encryptedSecret string
	var recoveryHashes []string
	err := a.db.QueryRow(r.Context(), `SELECT password_hash, COALESCE(mfa_secret, ''), mfa_recovery_hashes FROM operator_users WHERE id=$1`, identity.UserID).Scan(&passwordHash, &encryptedSecret, &recoveryHashes)
	if err != nil {
		a.logError(r, "load mfa secret for disable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_current_password", "The current password is incorrect.")
		return
	}
	if encryptedSecret == "" {
		writeError(w, http.StatusConflict, "mfa_not_enabled", "Two-factor authentication is not enabled.")
		return
	}
	secret, err := a.decryptMFASecret(encryptedSecret)
	if err != nil {
		a.logError(r, "decrypt mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	if !totp.Validate(secret, input.Code, time.Now()) && !containsRecoveryCode(recoveryHashes, input.Code) {
		writeError(w, http.StatusBadRequest, "invalid_mfa_code", "The authenticator or recovery code is not valid.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin mfa disable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE operator_users SET mfa_secret=NULL, mfa_pending_secret=NULL, mfa_recovery_hashes='{}', mfa_pending_recovery_hashes='{}', mfa_enabled_at=NULL WHERE id=$1`, identity.UserID); err != nil {
		a.logError(r, "disable mfa failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	if err := recordOperatorAudit(r, tx, identity.TenantID, "MFA_DISABLED", identity.UserID, map[string]string{"username": identity.Username}); err != nil {
		a.logError(r, "record mfa disable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit mfa disable failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "MFA could not be disabled.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) operatorLoginMFA(w http.ResponseWriter, r *http.Request) {
	if allowed, retry := a.allowRequest(r, "operator-login-mfa", 20, time.Minute); !allowed {
		setRetryAfter(w, retry)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many verification attempts. Please wait and try again.")
		return
	}
	var input struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter the authenticator code.")
		return
	}
	claims, valid := verifyMFAChallenge(a.cfg.OperatorTokenKey, input.ChallengeToken)
	if !valid {
		writeError(w, http.StatusUnauthorized, "mfa_challenge_invalid", "The sign-in challenge is invalid or expired. Sign in again.")
		return
	}

	var identity operatorIdentity
	var tokenVersion int
	var encryptedSecret string
	var recoveryHashes []string
	err := a.db.QueryRow(r.Context(), `
		SELECT u.id, u.username, u.display_name, COALESCE(u.platform_role, ''), u.token_version,
		       m.id, m.tenant_id, t.name, t.slug, m.role, m.status,
		       COALESCE(ja.id, ''), COALESCE(ja.status, ''),
		       COALESCE(u.mfa_secret, ''), u.mfa_recovery_hashes
		FROM operator_users u
		JOIN tenant_memberships m ON m.user_id=u.id AND m.status IN ('PENDING', 'ACTIVE')
		JOIN tenants t ON t.id=m.tenant_id AND t.status='ACTIVE'
		LEFT JOIN join_applications ja ON ja.operator_user_id=u.id
		WHERE u.id=$1 AND u.status='ACTIVE'
		ORDER BY CASE WHEN t.id='tenant-default' THEN 0 ELSE 1 END, m.created_at
		LIMIT 1`, claims.UserID).Scan(
		&identity.UserID, &identity.Username, &identity.DisplayName, &identity.PlatformRole, &tokenVersion,
		&identity.MembershipID, &identity.TenantID, &identity.TenantName, &identity.TenantSlug, &identity.Role, &identity.MembershipStatus,
		&identity.ApplicationID, &identity.ApplicationStatus, &encryptedSecret, &recoveryHashes,
	)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "mfa_challenge_invalid", "The sign-in challenge is invalid or expired. Sign in again.")
		return
	}
	if encryptedSecret == "" {
		writeError(w, http.StatusUnauthorized, "mfa_challenge_invalid", "Two-factor authentication is not enabled for this account anymore. Sign in again.")
		return
	}
	secret, err := a.decryptMFASecret(encryptedSecret)
	if err != nil {
		a.logError(r, "decrypt mfa secret failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The sign-in challenge could not be verified.")
		return
	}
	recoveryIndex := -1
	if !totp.Validate(secret, input.Code, time.Now()) {
		recoveryIndex = recoveryCodeIndex(recoveryHashes, input.Code)
		if recoveryIndex < 0 {
			writeError(w, http.StatusUnauthorized, "invalid_mfa_code", "The authenticator code is not valid. Check your device clock and try again.")
			return
		}
	}
	if recoveryIndex >= 0 {
		remaining := append(append([]string{}, recoveryHashes[:recoveryIndex]...), recoveryHashes[recoveryIndex+1:]...)
		if _, err := a.db.Exec(r.Context(), `UPDATE operator_users SET mfa_recovery_hashes=$2 WHERE id=$1`, identity.UserID, remaining); err != nil {
			a.logError(r, "consume recovery code failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The sign-in challenge could not be verified.")
			return
		}
		a.recordOperatorAuditSimple(r, identity.TenantID, "MFA_RECOVERY_USED", identity.UserID, identity.Username)
	}
	a.completeOperatorLogin(w, r, identity, tokenVersion)
}

func newMFAChallenge(secret []byte, userID string) (string, time.Time, error) {
	nonce, err := newOpaqueToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(mfaChallengeTTL).UTC()
	payload, err := json.Marshal(mfaChallengeClaims{Version: 1, UserID: userID, ExpiresAt: expiresAt.Unix(), Nonce: nonce, Purpose: "mfa"})
	if err != nil {
		return "", time.Time{}, err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return token, expiresAt, nil
}

func verifyMFAChallenge(secret []byte, token string) (mfaChallengeClaims, bool) {
	var claims mfaChallengeClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, false
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, false
	}
	valid := claims.Version == 1 && claims.UserID != "" && claims.Nonce != "" && claims.Purpose == "mfa" && time.Now().Before(time.Unix(claims.ExpiresAt, 0))
	return claims, valid
}

func (a *App) mfaKey() []byte {
	sum := sha256.Sum256(append([]byte("nover-mfa-v1:"), a.cfg.OperatorTokenKey...))
	return sum[:]
}

func (a *App) encryptMFASecret(secret string) (string, error) {
	block, err := aes.NewCipher(a.mfaKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), []byte("nover-mfa"))
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (a *App) decryptMFASecret(value string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(a.mfaKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("mfa secret is malformed")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("nover-mfa"))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func newRecoveryCode() (string, error) {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	encoded := recoveryEncoding.EncodeToString(raw[:])
	encoded = strings.ToUpper(encoded)
	if len(encoded) > 12 {
		encoded = encoded[:12]
	}
	return encoded[0:4] + "-" + encoded[4:8] + "-" + encoded[8:12], nil
}

func normalizeRecoveryCode(code string) string {
	var builder strings.Builder
	for _, character := range strings.ToUpper(strings.TrimSpace(code)) {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte("nover-recovery:" + normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

func containsRecoveryCode(hashes []string, code string) bool {
	return recoveryCodeIndex(hashes, code) >= 0
}

func recoveryCodeIndex(hashes []string, code string) int {
	candidate := hashRecoveryCode(code)
	for index, hash := range hashes {
		if hmac.Equal([]byte(hash), []byte(candidate)) {
			return index
		}
	}
	return -1
}

func recordOperatorAudit(r *http.Request, tx pgx.Tx, tenantID, action, resourceID string, metadata map[string]string) error {
	eventID, err := randomID()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,$4,'operator_user',$5,$6::jsonb)`, eventID, tenantID, operatorActor(r), action, resourceID, string(payload))
	return err
}

func (a *App) recordOperatorAuditSimple(r *http.Request, tenantID, action, resourceID, username string) {
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		return
	}
	defer tx.Rollback(r.Context())
	if err := recordOperatorAudit(r, tx, tenantID, action, resourceID, map[string]string{"username": username}); err == nil {
		_ = tx.Commit(r.Context())
	}
}
