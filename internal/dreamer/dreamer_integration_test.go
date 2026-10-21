//go:build integration

package dreamer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/store/storetest"
)

func TestHeartbeatIsWorked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := storetest.Open(t).App // the service role, as in production

	// Enqueue a heartbeat for a fixed past minute and wait for the worker to
	// complete it; RunOnStart also inserts one for the current minute.
	own := jobs.HeartbeatFor(time.Date(1999, 12, 31, 23, 0, 0, 0, time.UTC))
	if _, err := pool.Jobs().Insert(ctx, own); err != nil {
		t.Fatal(err)
	}
	minute := own.ScheduledFor.Format(time.RFC3339)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d, err := New(log, pool, testSchedule)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx) }()

	deadline := time.Now().Add(90 * time.Second)
	for {
		var n int
		err := pool.Pgx().QueryRow(ctx,
			"select count(*) from river_job where kind = $1 and state = 'completed' and args->>'scheduled_for' = $2",
			jobs.KindHeartbeat, minute).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat not completed within the deadline")
		}
		time.Sleep(500 * time.Millisecond)
	}

	rec := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, body %s", rec.Code, rec.Body.String())
	}

	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
