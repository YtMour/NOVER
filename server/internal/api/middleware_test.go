package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YtMour/NOVER/server/internal/config"
)

func testApp() *App {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(nil, config.Config{Environment: "test", OperatorAPIPath: "cp-7f3c", AllowedOrigins: []string{"http://localhost:45173"}}, logger)
}

func TestHandlerAddsRequestIDHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	testApp().Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("X-Request-Id") == "" {
		t.Fatal("X-Request-Id header is missing")
	}
}

func TestMetaEndpointExposesBuildIdentity(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	response := httptest.NewRecorder()
	testApp().Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode meta response: %v", err)
	}
	if payload["name"] != "nover" || payload["version"] == "" || payload["environment"] != "test" {
		t.Fatalf("meta payload = %v", payload)
	}
}
