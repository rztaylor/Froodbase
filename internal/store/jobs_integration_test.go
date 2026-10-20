//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

func TestJobsEnqueueTransactionally(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool := storetest.Open(t).App // the service role, as in production
	minute := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	count := func() int {
		var n int
		if err := pool.Pgx().QueryRow(ctx, "select count(*) from river_job where kind = $1", jobs.KindHeartbeat).Scan(&n); err != nil {
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
	if depth, err := pool.QueueDepth(ctx); err != nil || depth != 1 {
		t.Fatalf("QueueDepth = %d, %v, want 1", depth, err)
	}
}
