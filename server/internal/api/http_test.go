package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDecodeJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "one object", body: `{"code":"NVR"}`, want: false},
		{name: "unknown field", body: `{"code":"NVR","extra":true}`, want: true},
		{name: "trailing object", body: `{"code":"NVR"} {"again":true}`, want: true},
		{name: "invalid JSON", body: `{`, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			var input struct {
				Code string `json:"code"`
			}
			gotErr := decodeJSON(recorder, request, &input) != nil
			if gotErr != test.want {
				t.Fatalf("decodeJSON error = %v, want error %v", gotErr, test.want)
			}
		})
	}
}

func TestClientAddressDropsEphemeralPort(t *testing.T) {
	if got := clientAddress("192.0.2.10:53124"); got != "192.0.2.10" {
		t.Fatalf("client address = %q", got)
	}
	if got := clientAddress("[2001:db8::10]:53124"); got != "2001:db8::10" {
		t.Fatalf("IPv6 client address = %q", got)
	}
}

func TestRateLimiterCountsSameHostAcrossPorts(t *testing.T) {
	limiter := newRateLimiter()
	first := clientAddress("192.0.2.10:53124")
	second := clientAddress("192.0.2.10:53125")
	if allowed, _ := limiter.allow("redeem:"+first, 1, time.Minute); !allowed {
		t.Fatal("first request should pass")
	}
	if allowed, _ := limiter.allow("redeem:"+second, 1, time.Minute); allowed {
		t.Fatal("new ephemeral port should share the same host limit")
	}
}

func TestRateLimiterReportsRetryAfter(t *testing.T) {
	limiter := newRateLimiter()
	window := 30 * time.Second
	if allowed, retry := limiter.allow("login:192.0.2.10", 1, window); !allowed || retry != 0 {
		t.Fatalf("first request allowed = %v retry = %v, want true and 0", allowed, retry)
	}
	allowed, retry := limiter.allow("login:192.0.2.10", 1, window)
	if allowed {
		t.Fatal("second request should be rate limited")
	}
	if retry <= 0 || retry > window {
		t.Fatalf("retry = %v, want a value in (0, %v]", retry, window)
	}
}

func TestSetRetryAfterWritesCeilingSeconds(t *testing.T) {
	recorder := httptest.NewRecorder()
	if seconds := setRetryAfter(recorder, 1500*time.Millisecond); seconds != 2 {
		t.Fatalf("seconds = %d, want 2", seconds)
	}
	if got := recorder.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
}
