// Package maintenance implements conservative data-retention cleanup for NOVER.
//
// The command is intentionally dry-run by default: it reports what would be
// removed and only touches data when Options.Apply is true. Business records
// (redeem codes, entitlements) are never deleted, only expired ones are marked
// EXPIRED so history and audit trails remain intact. Private asset files are
// deleted only when no database row references them and they are older than
// minFileAge, which protects in-flight uploads.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const minOrphanFileAge = 24 * time.Hour

// Options controls one maintenance run.
type Options struct {
	// Apply performs the changes. When false the run is a dry run.
	Apply bool
	// StorageDir is the private asset root. An empty value skips file cleanup.
	StorageDir string
	// Now is injectable for tests. Zero means time.Now().UTC().
	Now time.Time
}

// Summary is the machine-readable result of a maintenance run.
type Summary struct {
	DryRun              bool     `json:"dry_run"`
	Version             string   `json:"version,omitempty"`
	SessionsExpired     int64    `json:"sessions_expired"`
	SessionsRevoked     int64    `json:"sessions_revoked"`
	TicketsExpired      int64    `json:"tickets_expired"`
	CodesMarkedExpired  int64    `json:"codes_marked_expired"`
	EntitlementsExpired int64    `json:"entitlements_marked_expired"`
	OrphanFiles         int      `json:"orphan_files"`
	OrphanFileSamples   []string `json:"orphan_file_samples,omitempty"`
	EmptyDirectories    int      `json:"empty_directories"`
	StorageDir          string   `json:"storage_dir,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

// Run counts (and optionally removes) expired rows and unreferenced files.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (Summary, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	summary := Summary{DryRun: !opts.Apply, StorageDir: opts.StorageDir}

	type job struct {
		target *int64
		count  string
		apply  string
	}
	jobs := []job{
		{
			target: &summary.SessionsExpired,
			count:  `SELECT COUNT(*) FROM operator_sessions WHERE expires_at < NOW() - INTERVAL '1 day'`,
			apply:  `DELETE FROM operator_sessions WHERE expires_at < NOW() - INTERVAL '1 day'`,
		},
		{
			target: &summary.SessionsRevoked,
			count:  `SELECT COUNT(*) FROM operator_sessions WHERE revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days'`,
			apply:  `DELETE FROM operator_sessions WHERE revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days'`,
		},
		{
			target: &summary.TicketsExpired,
			count:  `SELECT COUNT(*) FROM download_tickets WHERE expires_at < NOW() - INTERVAL '1 day'`,
			apply:  `DELETE FROM download_tickets WHERE expires_at < NOW() - INTERVAL '1 day'`,
		},
		{
			target: &summary.CodesMarkedExpired,
			count:  `SELECT COUNT(*) FROM redeem_codes WHERE status = 'ASSIGNED' AND expires_at < NOW()`,
			apply:  `UPDATE redeem_codes SET status = 'EXPIRED' WHERE status = 'ASSIGNED' AND expires_at < NOW()`,
		},
		{
			target: &summary.EntitlementsExpired,
			count:  `SELECT COUNT(*) FROM entitlements WHERE status = 'ACTIVE' AND valid_until < NOW()`,
			apply:  `UPDATE entitlements SET status = 'EXPIRED' WHERE status = 'ACTIVE' AND valid_until < NOW()`,
		},
	}
	for _, item := range jobs {
		count, err := countAndApply(ctx, pool, opts.Apply, item.count, item.apply)
		if err != nil {
			return summary, err
		}
		*item.target = count
	}

	if strings.TrimSpace(opts.StorageDir) == "" {
		summary.Warnings = append(summary.Warnings, "storage cleanup skipped: storage directory is empty")
		return summary, nil
	}

	referenced, err := loadReferencedObjectKeys(ctx, pool)
	if err != nil {
		return summary, err
	}
	orphanFiles, emptyDirs, err := scanStorage(opts.StorageDir, referenced, now, minOrphanFileAge)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			summary.Warnings = append(summary.Warnings, "storage cleanup skipped: storage directory does not exist")
			return summary, nil
		}
		return summary, err
	}
	summary.OrphanFiles = len(orphanFiles)
	summary.EmptyDirectories = len(emptyDirs)
	summary.OrphanFileSamples = first(orphanFiles, 10)

	if !opts.Apply {
		return summary, nil
	}
	if err := deleteOrphans(opts.StorageDir, orphanFiles); err != nil {
		return summary, err
	}
	removeEmptyDirectories(opts.StorageDir, emptyDirs)
	return summary, nil
}

func countAndApply(ctx context.Context, pool *pgxpool.Pool, apply bool, countSQL, applySQL string) (int64, error) {
	var count int64
	if err := pool.QueryRow(ctx, countSQL).Scan(&count); err != nil {
		return 0, fmt.Errorf("count maintenance rows: %w", err)
	}
	if !apply || count == 0 {
		return count, nil
	}
	tag, err := pool.Exec(ctx, applySQL)
	if err != nil {
		return 0, fmt.Errorf("apply maintenance change: %w", err)
	}
	return tag.RowsAffected(), nil
}

func loadReferencedObjectKeys(ctx context.Context, pool *pgxpool.Pool) (map[string]struct{}, error) {
	rows, err := pool.Query(ctx, `SELECT object_key FROM assets UNION SELECT object_key FROM product_images`)
	if err != nil {
		return nil, fmt.Errorf("load referenced object keys: %w", err)
	}
	defer rows.Close()
	keys := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan object key: %w", err)
		}
		keys[normalizeKey(key)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object keys: %w", err)
	}
	return keys, nil
}

// scanStorage returns unreferenced files older than minAge and directories that
// are currently empty. Paths are relative to root and slash-separated.
func scanStorage(root string, referenced map[string]struct{}, now time.Time, minAge time.Duration) ([]string, []string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve storage directory: %w", err)
	}
	info, err := os.Stat(rootAbs)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("storage path %s is not a directory", rootAbs)
	}

	orphans := make([]string, 0)
	directories := make([]string, 0)
	err = filepath.WalkDir(rootAbs, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == rootAbs {
			return nil
		}
		relative, err := filepath.Rel(rootAbs, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			directories = append(directories, relative)
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if _, ok := referenced[normalizeKey(relative)]; ok {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(fileInfo.ModTime()) < minAge {
			return nil
		}
		orphans = append(orphans, relative)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(orphans)

	// Deepest directories first so parents can be removed in the same pass.
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(directories[i], "/")
		rightDepth := strings.Count(directories[j], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[i] < directories[j]
	})
	emptyDirs := make([]string, 0, len(directories))
	for _, directory := range directories {
		full, err := safeJoin(rootAbs, directory)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(full)
		if err == nil && len(entries) == 0 {
			emptyDirs = append(emptyDirs, directory)
		}
	}
	return orphans, emptyDirs, nil
}

func deleteOrphans(root string, files []string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve storage directory: %w", err)
	}
	for _, relative := range files {
		full, err := safeJoin(rootAbs, relative)
		if err != nil {
			return err
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete orphan file %s: %w", relative, err)
		}
	}
	return nil
}

func removeEmptyDirectories(root string, directories []string) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	for _, relative := range directories {
		full, err := safeJoin(rootAbs, relative)
		if err != nil {
			continue
		}
		// Ignore failures: a directory may have received new files after the scan.
		_ = os.Remove(full)
	}
}

func safeJoin(rootAbs, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("refuse absolute path %q", relative)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refuse path outside storage root %q", relative)
	}
	full := filepath.Join(rootAbs, clean)
	back, err := filepath.Rel(rootAbs, full)
	if err != nil || back == "." || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refuse path outside storage root %q", relative)
	}
	return full, nil
}

func normalizeKey(key string) string {
	return strings.TrimPrefix(strings.ReplaceAll(key, "\\", "/"), "/")
}

func first(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
