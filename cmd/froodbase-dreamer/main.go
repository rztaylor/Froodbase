package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"

	"github.com/rztaylor/froodbase/internal/config"
	"github.com/rztaylor/froodbase/internal/dreamer"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/jobs"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run works background jobs and serves the health endpoint until a signal
// arrives; "rebuild-index <area-id>" enqueues a rebuild of one area's
// projections and "review-sweep" a review sweep of every area now, and
// exit. Exit codes: 0 ok, 1 runtime failure, 2 bad configuration or usage.
func run(args []string) int {
	if len(args) > 0 && args[0] == "rebuild-index" {
		return runRebuildIndex(args[1:])
	}
	if len(args) > 0 && args[0] == "review-sweep" {
		return runReviewSweep(args[1:])
	}
	flags := flag.NewFlagSet("froodbase-dreamer", flag.ContinueOnError)
	showVersion := flags.Bool("version", false, "print the build version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("froodbase-dreamer", version.String())
		return 0
	}

	cfg, err := config.LoadDreamer(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-dreamer:", err)
		return 2
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("froodbase-dreamer failed", "err", err)
		return 1
	}
	defer pool.Close()

	d, err := dreamer.New(log, pool, dreamer.Schedule{ReviewSweepInterval: cfg.ReviewSweepInterval})
	if err != nil {
		log.Error("froodbase-dreamer failed", "err", err)
		return 1
	}
	log.Info("froodbase-dreamer starting", "version", version.Version, "health_addr", cfg.Addr)

	errc := make(chan error, 2)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { errc <- d.ServeHealth(runCtx, cfg.Addr) }()
	go func() { errc <- d.Run(runCtx) }()

	// The first failure stops the other; a clean signal-driven stop yields nil.
	err = <-errc
	cancel()
	if err2 := <-errc; err == nil {
		err = err2
	}
	log.Info("froodbase-dreamer stopping")
	if err != nil {
		log.Error("froodbase-dreamer failed", "err", err)
		return 1
	}
	return 0
}

func runRebuildIndex(args []string) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "usage: froodbase-dreamer rebuild-index <area-id>")
		return 2
	}
	return enqueue("rebuild-index", jobs.RebuildIndexArgs{AreaID: args[0]},
		"a rebuild for this area is already waiting", " for area "+args[0])
}

func runReviewSweep(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: froodbase-dreamer review-sweep")
		return 2
	}
	// The exact time is its own slot, so it never merges with a scheduled sweep.
	return enqueue("review-sweep", jobs.ReviewSweepArgs{ScheduledFor: time.Now().UTC()},
		"a sweep for this moment is already waiting", " for every area")
}

// enqueue inserts one job for a running dreamer to work and reports it.
func enqueue(command string, job river.JobArgs, duplicate, target string) int {
	cfg, err := config.LoadDreamer(os.LookupEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "froodbase-dreamer %s: %v\n", command, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "froodbase-dreamer %s: %v\n", command, err)
		return 1
	}
	defer pool.Close()
	res, err := pool.Jobs().Insert(ctx, job)
	if err != nil {
		fmt.Fprintf(os.Stderr, "froodbase-dreamer %s: %v\n", command, err)
		return 1
	}
	if res.UniqueSkippedAsDuplicate {
		fmt.Printf("%s: %s\n", command, duplicate)
		return 0
	}
	fmt.Printf("%s: enqueued job %d%s\n", command, res.Job.ID, target)
	return 0
}
