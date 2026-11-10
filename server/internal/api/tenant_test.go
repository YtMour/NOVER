package api

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestRequestTenantIDUsesVerifiedSessionTenant(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	if got := requestTenantID(request); got != defaultTenantID {
		t.Fatalf("public tenant = %q, want %q", got, defaultTenantID)
	}

	identity := operatorIdentity{TenantID: "tenant-acme"}
	request = request.WithContext(context.WithValue(request.Context(), operatorContextKey{}, identity))
	if got := requestTenantID(request); got != "tenant-acme" {
		t.Fatalf("operator tenant = %q, want tenant-acme", got)
	}
}

func TestRequestTenantIDIgnoresEmptySessionTenant(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil).WithContext(
		context.WithValue(context.Background(), operatorContextKey{}, operatorIdentity{}),
	)
	if got := requestTenantID(request); got != defaultTenantID {
		t.Fatalf("empty session tenant = %q, want %q", got, defaultTenantID)
	}
}
