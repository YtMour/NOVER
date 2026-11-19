// Package totp implements RFC 6238 time-based one-time passwords with the
// parameters used by mainstream authenticator apps (SHA-1, 6 digits, 30s).
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	digits = 6
	period = 30 * time.Second
	// Allow one period of clock drift in either direction.
	window = 1
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new 20-byte secret encoded as base32 (160 bits),
// which is the size recommended by RFC 4226.
func GenerateSecret() (string, error) {
	var raw [20]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return encoding.EncodeToString(raw[:]), nil
}

// URI builds an otpauth:// URL that authenticator apps can import.
func URI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprintf("%d", digits))
	query.Set("period", fmt.Sprintf("%d", int(period.Seconds())))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// Validate checks a user-supplied code against the secret with a one-step
// window. Invalid inputs and malformed codes return false.
func Validate(secret, code string, now time.Time) bool {
	code = normalizeCode(code)
	if len(code) != digits {
		return false
	}
	secretKey, err := decodeSecret(secret)
	if err != nil {
		return false
	}
	counter := now.UTC().Unix() / int64(period.Seconds())
	for offset := int64(-window); offset <= int64(window); offset++ {
		candidate := generate(secretKey, counter+offset)
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// CodeAt returns the current code for tests and tooling. Production code
// should prefer Validate.
func CodeAt(secret string, now time.Time) (string, error) {
	secretKey, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	counter := now.UTC().Unix() / int64(period.Seconds())
	return generate(secretKey, counter), nil
}

func normalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	var digitsOnly strings.Builder
	for _, character := range code {
		if character >= '0' && character <= '9' {
			digitsOnly.WriteRune(character)
		}
	}
	return digitsOnly.String()
}

func decodeSecret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	return encoding.DecodeString(normalized)
}

func generate(secret []byte, counter int64) string {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(buffer[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (int32(sum[offset])&0x7f)<<24 |
		int32(sum[offset+1])<<16 |
		int32(sum[offset+2])<<8 |
		int32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
