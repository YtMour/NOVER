package api

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestNewRedeemCodeFormatAndNormalization(t *testing.T) {
	code, err := newRedeemCode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(code, "NVR-") {
		t.Fatalf("code prefix = %q, want NVR-", code)
	}
	normalized := normalizeCode(code)
	if len(normalized) != 26 {
		t.Fatalf("normalized code length = %d, want 26", len(normalized))
	}
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized); err != nil {
		t.Fatalf("normalized code is not valid base32: %v", err)
	}
	if got := normalizeCode(strings.ToLower(code)); got != normalized {
		t.Fatalf("normalization should ignore case: %q != %q", got, normalized)
	}
	if !validRedeemCode(code) || !validRedeemCode(strings.ToLower(code)) {
		t.Fatal("generated redemption code should validate regardless of letter case")
	}
	if validRedeemCode("NVR-0000-0000-0000-0000-0000-0000-AA") {
		t.Fatal("characters outside the Base32 alphabet should fail validation")
	}
}

func TestRedeemCodeHashAndAccessToken(t *testing.T) {
	secret := []byte("test-redeem-hmac-key-with-32-bytes-min")
	accessKey := []byte("test-access-token-key-with-32-bytes-min")
	code := "NVR-ABCD-EFGH-IJKL-MNOP-QRST-UVWX-YZ"
	hash := hashCode(secret, code)
	if got := hashCode(secret, "abcd-efgh-ijkl-mnop-qrst-uvwx-yz"); string(got) != string(hash) {
		t.Fatal("same normalized code should have the same HMAC")
	}
	first := newAccessToken(accessKey, "entitlement-id", hash)
	second := newAccessToken(accessKey, "entitlement-id", hash)
	if first != second {
		t.Fatal("same entitlement should recover the same access token")
	}
	if first == newAccessToken(accessKey, "another-entitlement", hash) {
		t.Fatal("different entitlements should receive different access tokens")
	}
}

func TestOperatorTokenVerification(t *testing.T) {
	secret := []byte("operator-token-secret-with-32-bytes-min")
	token, expiresAt, err := newOperatorToken(secret, "operator", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("operator token should expire in the future")
	}
	if !verifyOperatorToken(secret, token, "operator") {
		t.Fatal("valid operator token was rejected")
	}
	if verifyOperatorToken(secret, token, "other-operator") {
		t.Fatal("token was accepted for another operator")
	}
	if verifyOperatorToken([]byte("another-secret-with-32-bytes-minimum"), token, "operator") {
		t.Fatal("token was accepted with another signing secret")
	}
}
