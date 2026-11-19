// Command maintenance reports and applies safe data-retention cleanup for a
// NOVER deployment.
//
// Examples:
//
//	# dry run (default): report what would change
//	go run ./cmd/maintenance
//
//	# inside Docker Compose (uses the ops profile)
//	docker compose --profile ops run --rm maintenance
//	docker compose --profile ops run --rm maintenance -apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YtMour/NOVER/server/internal/buildinfo"
	"github.com/YtMour/NOVER/server/internal/db"
	"github.com/YtMour/NOVER/server/internal/maintenance"
)

func main() {
	var (
		apply       bool
		jsonOutput  bool
		storageDir  string
		databaseURL string
		timeout     time.Duration
	)
	flag.BoolVar(&apply, "apply", false, "perform deletions/updates; omit for a dry run")
	flag.BoolVar(&jsonOutput, "json", false, "print the summary as JSON")
	flag.StringVar(&storageDir, "storage-dir", env("STORAGE_DIR", "./runtime-data/files"), "private asset storage root")
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string (default: DATABASE_URL)")
	flag.DurationVar(&timeout, "timeout", 5*time.Minute, "overall timeout for the run")
	flag.Parse()

	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "maintenance: DATABASE_URL or -database-url is required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	summary, err := maintenance.Run(ctx, pool, maintenance.Options{Apply: apply, StorageDir: storageDir, Now: time.Now().UTC()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "maintenance: %v\n", err)
		os.Exit(1)
	}
	summary.Version = buildinfo.Version

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(summary); err != nil {
			fmt.Fprintf(os.Stderr, "maintenance: encode summary: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printSummary(summary)
}

func printSummary(summary maintenance.Summary) {
	mode := "DRY RUN (no changes made; pass -apply to execute)"
	if !summary.DryRun {
		mode = "APPLIED"
	}
	fmt.Printf("NOVER maintenance — %s\n", mode)
	fmt.Printf("  version:                %s\n", summary.Version)
	fmt.Printf("  expired sessions:       %d\n", summary.SessionsExpired)
	fmt.Printf("  revoked sessions:       %d\n", summary.SessionsRevoked)
	fmt.Printf("  expired download links: %d\n", summary.TicketsExpired)
	fmt.Printf("  codes marked expired:   %d\n", summary.CodesMarkedExpired)
	fmt.Printf("  entitlements expired:   %d\n", summary.EntitlementsExpired)
	fmt.Printf("  orphan files:           %d\n", summary.OrphanFiles)
	fmt.Printf("  empty directories:      %d\n", summary.EmptyDirectories)
	if summary.StorageDir != "" {
		fmt.Printf("  storage root:           %s\n", summary.StorageDir)
	}
	for _, sample := range summary.OrphanFileSamples {
		fmt.Printf("  orphan sample:          %s\n", sample)
	}
	for _, warning := range summary.Warnings {
		fmt.Printf("  warning:                %s\n", warning)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
