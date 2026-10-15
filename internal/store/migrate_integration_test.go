//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := storetest.EmptyPool(t)

	first, riverFirst, err := pool.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || riverFirst == 0 {
		t.Fatalf("first run applied %v goose and %d River steps, want both > 0", first, riverFirst)
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
	if statuses[len(statuses)-1].Source != store.RiverSource {
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
