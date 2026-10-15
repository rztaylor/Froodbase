//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMigrateIsIdempotent runs against the database named by
// FROODBASE_TEST_DATABASE_URL and skips when it is unset. FRB-7 replaces this
// with the Testcontainers helper.
func TestMigrateIsIdempotent(t *testing.T) {
	url := os.Getenv("FROODBASE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FROODBASE_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, _, err := pool.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	again, riverAgain, err := pool.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 || riverAgain != 0 {
		t.Fatalf("second run applied %v and %d River steps, want nothing", again, riverAgain)
	}

	statuses, err := pool.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if !s.Applied {
			t.Errorf("migration %d (%s) not applied", s.Version, s.Source)
		}
	}
	if statuses[len(statuses)-1].Source != RiverSource {
		t.Errorf("last status = %+v, want the River entry", statuses[len(statuses)-1])
	}

	var n int
	err = pool.Pgx().QueryRow(ctx,
		"select count(*) from pg_extension where extname in ('vector', 'pg_trgm', 'btree_gist')").Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("installed extensions = %d, want 3", n)
	}
}
