package api

import (
	"strings"
	"testing"

	"github.com/YtMour/NOVER/server/internal/config"
)

func TestMFAChallengeRoundTrip(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	token, expiresAt, err := newMFAChallenge(key, "user-1")
	if err != nil {
		t.Fatalf("newMFAChallenge() error = %v", err)
	}
	if expiresAt.IsZero() || len(token) < 40 {
		t.Fatalf("challenge = %q expires = %v", token, expiresAt)
	}
	claims, valid := verifyMFAChallenge(key, token)
	if !valid || claims.UserID != "user-1" || claims.Purpose != "mfa" {
		t.Fatalf("verifyMFAChallenge() = %#v valid=%v", claims, valid)
	}
	if _, valid := verifyMFAChallenge([]byte(strings.Repeat("x", 32)), token); valid {
		t.Fatal("challenge verified with the wrong key")
	}
	parts := strings.Split(token, ".")
	tampered := parts[0] + "." + strings.Repeat("A", len(parts[1]))
	if _, valid := verifyMFAChallenge(key, tampered); valid {
		t.Fatal("tampered challenge was accepted")
	}
	if _, valid := verifyMFAChallenge(key, "not-a-token"); valid {
		t.Fatal("malformed challenge was accepted")
	}
}

func TestMFAEncryptionRoundTrip(t *testing.T) {
	app := &App{cfg: config.Config{OperatorTokenKey: []byte(strings.Repeat("s", 32))}}
	encrypted, err := app.encryptMFASecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encryptMFASecret() error = %v", err)
	}
	if encrypted == "JBSWY3DPEHPK3PXP" || strings.Contains(encrypted, "JBSW") {
		t.Fatal("secret was not encrypted")
	}
	decrypted, err := app.decryptMFASecret(encrypted)
	if err != nil || decrypted != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decryptMFASecret() = %q err=%v", decrypted, err)
	}
	if _, err := app.decryptMFASecret("not-base64"); err == nil {
		t.Fatal("malformed ciphertext was accepted")
	}
	if _, err := app.decryptMFASecret("AAAA"); err == nil {
		t.Fatal("short ciphertext was accepted")
	}
}

func TestRecoveryCodeHashing(t *testing.T) {
	code, err := newRecoveryCode()
	if err != nil {
		t.Fatalf("newRecoveryCode() error = %v", err)
	}
	parts := strings.Split(code, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) != 4 || len(parts[2]) != 4 {
		t.Fatalf("recovery code format = %q", code)
	}
	normalized := normalizeRecoveryCode(strings.ToLower(code))
	if len(normalized) != 12 {
		t.Fatalf("normalized recovery code = %q", normalized)
	}
	hashes := []string{hashRecoveryCode("AAAA-BBBB-CCCC"), hashRecoveryCode(code)}
	if index := recoveryCodeIndex(hashes, normalized); index != 1 {
		t.Fatalf("recoveryCodeIndex() = %d, want 1", index)
	}
	if index := recoveryCodeIndex(hashes, "ZZZZ-ZZZZ-ZZZZ"); index != -1 {
		t.Fatalf("unknown recovery code index = %d, want -1", index)
	}
	if !containsRecoveryCode(hashes, code) {
		t.Fatal("containsRecoveryCode() did not find the code")
	}
}

func TestInvitationRoleAndTokenHelpers(t *testing.T) {
	if !isAssignableMemberRole("EDITOR") || !isAssignableMemberRole("ANALYST") {
		t.Fatal("expected assignable roles to be accepted")
	}
	for _, role := range []string{"OWNER", "ROOT", ""} {
		if isAssignableMemberRole(role) {
			t.Fatalf("role %q should not be assignable", role)
		}
	}
	token, err := newInvitationToken()
	if err != nil || len(token) < 40 {
		t.Fatalf("invitation token = %q err=%v", token, err)
	}
	if len(hashToken(token)) != 32 {
		t.Fatal("token hash should be 32 bytes")
	}
}
