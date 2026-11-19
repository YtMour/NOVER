package buildinfo

import "testing"

func TestInfoIncludesInjectedValues(t *testing.T) {
	originalVersion, originalCommit, originalBuiltAt := Version, Commit, BuiltAt
	defer func() {
		Version, Commit, BuiltAt = originalVersion, originalCommit, originalBuiltAt
	}()

	Version, Commit, BuiltAt = "v0.2.0", "abc1234", "2026-11-19T05:00:00Z"
	info := Info()
	if info["version"] != "v0.2.0" || info["commit"] != "abc1234" || info["built_at"] != "2026-11-19T05:00:00Z" {
		t.Fatalf("Info() = %v, want injected build metadata", info)
	}
}
