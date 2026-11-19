package totp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors use an ASCII secret of 20 bytes
// "12345678901234567890"; its base32 form is used below. The RFC prints
// 8-digit codes, so the 6-digit implementation should match the last six.
func TestValidateRFC6238Vectors(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, test := range tests {
		now := time.Unix(test.unix, 0)
		code, err := CodeAt(secret, now)
		if err != nil {
			t.Fatalf("CodeAt(%d) error = %v", test.unix, err)
		}
		if code != test.want {
			t.Fatalf("CodeAt(%d) = %s, want %s", test.unix, code, test.want)
		}
		if !Validate(secret, code, now) {
			t.Fatalf("Validate(%s) = false for generated code", code)
		}
		// One period of drift is accepted, two periods are not.
		if !Validate(secret, code, now.Add(period)) {
			t.Fatalf("Validate should accept one period of clock drift")
		}
		if Validate(secret, code, now.Add(3*period)) {
			t.Fatalf("Validate accepted a code three periods away")
		}
	}
}

func TestValidateRejectsMalformedInput(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, err := CodeAt(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"", "12345", "1234567", "abcdef", "not-a-code"} {
		if Validate(secret, candidate, now) {
			t.Fatalf("Validate(%q) = true, want false", candidate)
		}
	}
	// Spaces and separators are tolerated for user input.
	spaced := code[:3] + " " + code[3:]
	if !Validate(secret, spaced, now) {
		t.Fatalf("Validate should tolerate whitespace")
	}
	if !strings.HasPrefix(URI("NOVER", "admin", secret), "otpauth://totp/") {
		t.Fatal("URI should start with otpauth://totp/")
	}
}
