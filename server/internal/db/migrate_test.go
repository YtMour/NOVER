package db

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedMigrationsAreOrderedAndChecksummed(t *testing.T) {
	items, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected at least baseline plus one incremental migration, got %d", len(items))
	}
	if items[0].Version != "0001" {
		t.Fatalf("first migration version = %q, want 0001", items[0].Version)
	}
	for _, item := range items {
		if item.SQL == "" {
			t.Fatalf("migration %s has empty SQL", item.Version)
		}
		if len(item.Checksum) != 64 {
			t.Fatalf("migration %s checksum length = %d, want 64", item.Version, len(item.Checksum))
		}
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].Version >= items[i].Version {
			t.Fatalf("migrations not strictly ordered: %s then %s", items[i-1].Version, items[i].Version)
		}
	}
}

func TestParseMigrationsSortsByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0002_second.sql": {Data: []byte("SELECT 2;")},
		"migrations/0001_first.sql":  {Data: []byte("SELECT 1;")},
		"migrations/0010_tenth.sql":  {Data: []byte("SELECT 10;")},
	}
	items, err := parseMigrations(fsys, "migrations")
	if err != nil {
		t.Fatalf("parseMigrations() error = %v", err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Version)
	}
	want := "0001,0002,0010"
	if strings.Join(got, ",") != want {
		t.Fatalf("version order = %v, want %s", got, want)
	}
}

func TestParseMigrationsValidatesNamesAndDuplicates(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{
			name: "invalid file name",
			files: fstest.MapFS{
				"migrations/schema.sql": {Data: []byte("SELECT 1;")},
			},
		},
		{
			name: "duplicate version",
			files: fstest.MapFS{
				"migrations/0001_first.sql":  {Data: []byte("SELECT 1;")},
				"migrations/0001_second.sql": {Data: []byte("SELECT 2;")},
			},
		},
		{
			name:  "empty directory",
			files: fstest.MapFS{"migrations/.keep": {Data: []byte("")}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseMigrations(test.files, "migrations"); err == nil {
				t.Fatal("parseMigrations() error = nil, want validation error")
			}
		})
	}
}

func TestParseMigrationsChecksumTracksContent(t *testing.T) {
	a := fstest.MapFS{"migrations/0001_a.sql": {Data: []byte("SELECT 1;")}}
	b := fstest.MapFS{"migrations/0001_a.sql": {Data: []byte("SELECT 2;")}}
	itemsA, err := parseMigrations(a, "migrations")
	if err != nil {
		t.Fatalf("parseMigrations(a) error = %v", err)
	}
	itemsB, err := parseMigrations(b, "migrations")
	if err != nil {
		t.Fatalf("parseMigrations(b) error = %v", err)
	}
	if itemsA[0].Checksum == itemsB[0].Checksum {
		t.Fatal("checksum did not change when migration content changed")
	}
}
