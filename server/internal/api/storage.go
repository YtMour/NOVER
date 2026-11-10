package api

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var storageSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var storageFilenamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// tenantObjectKey keeps newly written files in a tenant-specific namespace.
// Existing legacy keys remain readable so storage can be migrated lazily.
func tenantObjectKey(tenantID, collection, filename string) (string, error) {
	if !storageSegmentPattern.MatchString(tenantID) || !storageSegmentPattern.MatchString(collection) || !storageFilenamePattern.MatchString(filename) || filename == "." || filename == ".." {
		return "", fmt.Errorf("invalid storage key segment")
	}
	return filepath.ToSlash(filepath.Join("tenants", tenantID, collection, filename)), nil
}

func safeStoragePath(storageDir, objectKey string) (string, bool) {
	if filepath.IsAbs(objectKey) || strings.Contains(filepath.ToSlash(objectKey), "../") {
		return "", false
	}
	return filepath.Join(storageDir, filepath.FromSlash(objectKey)), true
}
