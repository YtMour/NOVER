// Command migrate reports and applies versioned database migrations without
// starting the API. It is intended for local upgrades, CI and operator tooling.
//
// Examples:
//
//	go run ./cmd/migrate -status
//	go run ./cmd/migrate -apply
//	docker compose --profile ops run --rm migrate -status
//	docker compose --profile ops run --rm migrate -apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/YtMour/NOVER/server/internal/db"
)

func main() {
	var (
		databaseURL string
		apply       bool
		statusOnly  bool
		jsonOutput  bool
		timeout     time.Duration
	)
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string (default: DATABASE_URL)")
	flag.BoolVar(&apply, "apply", false, "apply pending migrations")
	flag.BoolVar(&statusOnly, "status", true, "report applied and pending migrations (default)")
	flag.BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "operation timeout")
	flag.Parse()

	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "migrate: DATABASE_URL or -database-url is required")
		os.Exit(2)
	}
	_ = statusOnly

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	appliedNow := 0
	if apply {
		appliedNow, err = db.Migrate(ctx, pool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate: apply failed: %v\n", err)
			os.Exit(1)
		}
	}
	plan, err := db.Plan(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: status failed: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		payload := map[string]any{
			"applied_now": appliedNow,
			"applied":     plan.Applied,
			"pending":     plan.Pending,
			"latest":      plan.Latest,
			"items":       plan.Items,
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			fmt.Fprintf(os.Stderr, "migrate: encode summary: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("NOVER migrations: %d applied, %d pending", plan.Applied, plan.Pending)
	if appliedNow > 0 {
		fmt.Printf(" (%d applied in this run)", appliedNow)
	}
	fmt.Println()
	for _, item := range plan.Items {
		marker := "pending"
		detail := ""
		if item.Applied {
			marker = "applied"
			if item.AppliedAt != nil {
				detail = " " + item.AppliedAt.UTC().Format(time.RFC3339)
			}
		}
		fmt.Printf("  [%s] %s %s%s\n", marker, item.Version, item.Name, detail)
	}
	if plan.Latest != "" {
		fmt.Printf("latest applied: %s\n", plan.Latest)
	}
	if plan.Pending > 0 {
		fmt.Println("run `migrate -apply` or restart the API to apply pending migrations")
	}
}
