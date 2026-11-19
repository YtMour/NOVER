package maintenance

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestScanStorageFindsOnlyOldUnreferencedFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	fresh := now.Add(-1 * time.Hour)

	writeFile(t, filepath.Join(root, "tenants", "tenant-a", "assets", "old-orphan.blob"), old)
	writeFile(t, filepath.Join(root, "tenants", "tenant-a", "assets", "old-referenced.blob"), old)
	writeFile(t, filepath.Join(root, "tenants", "tenant-a", "assets", "fresh-orphan.blob"), fresh)
	writeFile(t, filepath.Join(root, "tenants", "tenant-b", "product-images", "referenced.png"), old)
	if err := os.MkdirAll(filepath.Join(root, "tenants", "tenant-c", "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	referenced := map[string]struct{}{
		"tenants/tenant-a/assets/old-referenced.blob":    {},
		"tenants/tenant-b/product-images/referenced.png": {},
	}
	orphans, emptyDirs, err := scanStorage(root, referenced, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("scanStorage() error = %v", err)
	}
	sort.Strings(orphans)
	if len(orphans) != 1 || orphans[0] != "tenants/tenant-a/assets/old-orphan.blob" {
		t.Fatalf("orphans = %v, want [tenants/tenant-a/assets/old-orphan.blob]", orphans)
	}
	foundEmpty := false
	for _, directory := range emptyDirs {
		if directory == "tenants/tenant-c/assets" {
			foundEmpty = true
		}
	}
	if !foundEmpty {
		t.Fatalf("emptyDirs = %v, want tenants/tenant-c/assets", emptyDirs)
	}
}

func TestDeleteOrphansRefusesEscapingPaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, outside, time.Now().Add(-48*time.Hour))

	if _, err := safeJoin(root, "../outside.txt"); err == nil {
		t.Fatal("safeJoin accepted a traversal path")
	}
	if _, err := safeJoin(root, outside); err == nil {
		t.Fatal("safeJoin accepted an absolute path")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should be untouched: %v", err)
	}
}

func TestDeleteOrphansRemovesListedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.blob"), time.Now().Add(-48*time.Hour))
	writeFile(t, filepath.Join(root, "b.blob"), time.Now().Add(-48*time.Hour))

	if err := deleteOrphans(root, []string{"a.blob"}); err != nil {
		t.Fatalf("deleteOrphans() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.blob")); !os.IsNotExist(err) {
		t.Fatal("a.blob was not deleted")
	}
	if _, err := os.Stat(filepath.Join(root, "b.blob")); err != nil {
		t.Fatal("b.blob should still exist")
	}
}

func TestNormalizeKey(t *testing.T) {
	if got := normalizeKey("\\tenants\\tenant-a\\assets\\file.blob"); got != "tenants/tenant-a/assets/file.blob" {
		t.Fatalf("normalizeKey() = %q", got)
	}
}

func writeFile(t *testing.T, path string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}
