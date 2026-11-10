package api

import "testing"

func TestPermissionMatrixSeparatesTenantAndPlatformRoles(t *testing.T) {
	owner := permissionSet(operatorIdentity{Role: "OWNER", PlatformRole: "PLATFORM_OWNER"})
	if _, ok := owner[permissionProductPublish]; !ok {
		t.Fatal("owner should publish products")
	}
	if _, ok := owner[permissionSettingsManage]; !ok {
		t.Fatal("platform owner should manage settings")
	}

	editor := permissionSet(operatorIdentity{Role: "EDITOR"})
	if _, ok := editor[permissionProductWrite]; !ok {
		t.Fatal("editor should edit product content")
	}
	if _, ok := editor[permissionProductPublish]; ok {
		t.Fatal("editor should not publish products")
	}
	if _, ok := editor[permissionApplicationRead]; ok {
		t.Fatal("tenant editor should not see platform applications")
	}

	auditor := permissionSet(operatorIdentity{Role: "ANALYST", PlatformRole: "PLATFORM_AUDITOR"})
	if _, ok := auditor[permissionSettingsRead]; !ok {
		t.Fatal("platform auditor should read settings")
	}
	if _, ok := auditor[permissionSettingsManage]; ok {
		t.Fatal("platform auditor should not manage settings")
	}
}

func TestPendingMembershipHasNoBusinessPermissions(t *testing.T) {
	permissions := permissionSet(operatorIdentity{Role: "OWNER", MembershipStatus: "PENDING"})
	if len(permissions) != 0 {
		t.Fatalf("pending membership permissions = %#v, want empty", permissions)
	}
}

func TestAllowedOriginMatchesExplicitListOnly(t *testing.T) {
	allowed := []string{"https://store.example.com", "https://console.example.com"}
	if !allowedOrigin(allowed, "https://console.example.com") {
		t.Fatal("configured console origin was rejected")
	}
	if allowedOrigin(allowed, "https://sub.console.example.com") {
		t.Fatal("unconfigured subdomain origin was accepted")
	}
}
