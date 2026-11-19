package api

import (
	"net/http/httptest"
	"testing"
)

func TestParsePaginationDefaults(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/test", nil)
	limit, offset, err := parsePagination(request, 100, 200)
	if err != nil {
		t.Fatalf("parsePagination() error = %v", err)
	}
	if limit != 100 || offset != 0 {
		t.Fatalf("limit = %d offset = %d, want 100 and 0", limit, offset)
	}
}

func TestParsePaginationAcceptsExplicitValues(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/test?limit=25&offset=50", nil)
	limit, offset, err := parsePagination(request, 100, 200)
	if err != nil {
		t.Fatalf("parsePagination() error = %v", err)
	}
	if limit != 25 || offset != 50 {
		t.Fatalf("limit = %d offset = %d, want 25 and 50", limit, offset)
	}
}

func TestParsePaginationRejectsInvalidValues(t *testing.T) {
	for _, target := range []string{
		"/api/v1/test?limit=0",
		"/api/v1/test?limit=201",
		"/api/v1/test?limit=abc",
		"/api/v1/test?offset=-1",
		"/api/v1/test?offset=1.5",
	} {
		request := httptest.NewRequest("GET", target, nil)
		if _, _, err := parsePagination(request, 100, 200); err == nil {
			t.Fatalf("parsePagination(%q) error = nil, want validation error", target)
		}
	}
}
