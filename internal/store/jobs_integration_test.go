//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store/jobs"
)

func TestJobsEnqueueTransactionally(t *testing.T) {
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
	// A distinct minute far in the past so the test never collides with the
	// dreamer's live heartbeat.
	minute := time.Date(2000, 1, 2, 3, 4, 0, 0, time.UTC).Add(time.Duration(time.Now().UnixNano()%100000) * time.Minute)
	count := func() int {
		var n int
		if err := pool.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1 and args->>'scheduled_for' = $2",
			jobs.KindHeartbeat, minute.Format(time.RFC3339)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Rolled back transaction leaves no job.
	tx, err := pool.Pgx().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Jobs().InsertTx(ctx, tx, jobs.HeartbeatFor(minute)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 0 {
		t.Fatalf("after rollback: %d jobs, want 0", n)
	}

	// Committed transaction leaves one; a duplicate in the same minute is skipped.
	tx, err = pool.Pgx().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Jobs().InsertTx(ctx, tx, jobs.HeartbeatFor(minute)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := pool.Jobs().Insert(ctx, jobs.HeartbeatFor(minute))
	if err != nil {
		t.Fatal(err)
	}
	if !res.UniqueSkippedAsDuplicate {
		t.Error("second heartbeat for the same minute was not skipped as a duplicate")
	}
	if n := count(); n != 1 {
		t.Fatalf("after commit: %d jobs, want 1", n)
	}
	if depth, err := pool.QueueDepth(ctx); err != nil || depth < 1 {
		t.Fatalf("QueueDepth = %d, %v", depth, err)
	}
}
