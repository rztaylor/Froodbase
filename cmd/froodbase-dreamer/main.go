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
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("froodbase-dreamer", version.String())
		return
	}

	cfg, err := config.LoadDreamer(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-dreamer:", err)
		os.Exit(2)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("froodbase-dreamer failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Job workers arrive with later slice 0 tickets; until then the dreamer
	// only reports that it started and waits to be stopped.
	log.Info("froodbase-dreamer starting", "version", version.Version)
	<-ctx.Done()
	log.Info("froodbase-dreamer stopping")
}
