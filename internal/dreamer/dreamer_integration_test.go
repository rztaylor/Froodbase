//go:build integration

package dreamer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
)

func TestHeartbeatIsWorked(t *testing.T) {
	url := os.Getenv("FROODBASE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FROODBASE_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, _, err := pool.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	// The database may be shared with a running dreamer, whose own heartbeat
	// would satisfy a wait on the current minute. Enqueue a heartbeat for a
	// minute no scheduler produces and wait for a worker to complete it.
	own := jobs.HeartbeatFor(time.Date(1999, 12, 31, 23, 0, 0, 0, time.UTC).Add(time.Duration(time.Now().UnixNano()%100000) * time.Minute))
	if _, err := pool.Jobs().Insert(ctx, own); err != nil {
		t.Fatal(err)
	}
	minute := own.ScheduledFor.Format(time.RFC3339)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d, err := New(log, pool)
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
