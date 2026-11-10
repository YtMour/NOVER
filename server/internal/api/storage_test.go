package api

import "testing"

func TestTenantObjectKeyNamespacesNewFiles(t *testing.T) {
	got, err := tenantObjectKey("tenant-abc", "assets", "asset-id.blob")
	if err != nil {
		t.Fatal(err)
	}
	if got != "tenants/tenant-abc/assets/asset-id.blob" {
		t.Fatalf("object key = %q", got)
	}
	if _, err := tenantObjectKey("tenant/other", "assets", "asset.blob"); err == nil {
		t.Fatal("unsafe tenant segment was accepted")
	}
}

func TestSafeStoragePathRejectsTraversal(t *testing.T) {
	if _, ok := safeStoragePath("/srv/nover", "../outside"); ok {
		t.Fatal("path traversal was accepted")
	}
	if path, ok := safeStoragePath("/srv/nover", "tenants/tenant-a/assets/a.blob"); !ok || path == "" {
		t.Fatalf("valid storage path rejected: %q", path)
	}
}
