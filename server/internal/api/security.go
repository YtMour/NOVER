package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func newRedeemCode() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	encoded := codeEncoding.EncodeToString(raw[:])
	parts := make([]string, 0, 7)
	for len(encoded) > 4 {
		parts = append(parts, encoded[:4])
		encoded = encoded[4:]
	}
	if encoded != "" {
		parts = append(parts, encoded)
	}
	return "NVR-" + strings.Join(parts, "-"), nil
}

func normalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.TrimPrefix(code, "NVR-")
	return strings.ReplaceAll(code, "-", "")
}

func validRedeemCode(code string) bool {
	normalized := normalizeCode(code)
	if len(normalized) != 26 {
		return false
	}
	_, err := codeEncoding.DecodeString(normalized)
	return err == nil
}

func hashCode(secret []byte, code string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(normalizeCode(code)))
	return mac.Sum(nil)
}

func newAccessToken(accessKey []byte, entitlementID string, codeHash []byte) string {
	mac := hmac.New(sha256.New, accessKey)
	_, _ = mac.Write([]byte("entitlement:" + entitlementID + ":" + base64.RawURLEncoding.EncodeToString(codeHash)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newOpaqueToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func newOperatorToken(secret []byte, username string, ttl time.Duration) (string, time.Time, error) {
	nonce, err := newOpaqueToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(ttl).UTC()
	payload := fmt.Sprintf("v1|%s|%d|%s", username, expiresAt.Unix(), nonce)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return token, expiresAt, nil
}

func verifyOperatorToken(secret []byte, token, expectedUsername string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return false
	}
	fields := strings.Split(string(payload), "|")
	if len(fields) != 4 || fields[0] != "v1" || fields[1] != expectedUsername {
		return false
	}
	expiresUnix, err := strconv.ParseInt(fields[2], 10, 64)
	return err == nil && time.Now().Before(time.Unix(expiresUnix, 0))
}
