//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/rztaylor/froodbase/internal/store"
)

func TestPoolIsMigratedAndIsolated(t *testing.T) {
	ctx := context.Background()
	a := Pool(t)
	b := Pool(t)

	var exts int
	if err := a.Pgx().QueryRow(ctx,
		"select count(*) from pg_extension where extname in ('vector', 'pg_trgm', 'btree_gist')").Scan(&exts); err != nil {
		t.Fatal(err)
	}
	if exts != 3 {
		t.Fatalf("extensions installed = %d, want 3", exts)
	}

	statuses, err := a.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if !s.Applied {
			t.Errorf("migration %d (%s) not applied in the cloned database", s.Version, s.Source)
		}
	}
	if statuses[len(statuses)-1].Source != store.RiverSource {
		t.Errorf("River schema missing from status: %+v", statuses)
	}

	// Isolation: a table created in one database is invisible in the other.
	if _, err := a.Pgx().Exec(ctx, "create table isolation_probe (id int)"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.Pgx().QueryRow(ctx, "select count(*) from pg_tables where tablename = 'isolation_probe'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("databases handed to two tests share state")
	}
}

func TestEmptyPoolHasNoSchema(t *testing.T) {
	ctx := context.Background()
	p := EmptyPool(t)
	var n int
	if err := p.Pgx().QueryRow(ctx, "select count(*) from pg_tables where schemaname = 'public'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty database has %d public tables", n)
	}
}
