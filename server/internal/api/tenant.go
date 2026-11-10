package api

import "net/http"

// defaultTenantID is used by public storefront requests. Operator requests
// always resolve the tenant from the verified server-side session instead.
const defaultTenantID = "tenant-default"

func requestTenantID(r *http.Request) string {
	if identity, ok := operatorIdentityFromContext(r.Context()); ok && identity.TenantID != "" {
		return identity.TenantID
	}
	return defaultTenantID
}
