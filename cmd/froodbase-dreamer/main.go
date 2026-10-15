package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rztaylor/froodbase/internal/config"
	"github.com/rztaylor/froodbase/internal/dreamer"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run works background jobs and serves the health endpoint until a signal
// arrives. Exit codes: 0 ok, 1 runtime failure, 2 bad configuration.
func run(args []string) int {
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

	d, err := dreamer.New(log, pool)
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
