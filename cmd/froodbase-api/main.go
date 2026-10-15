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
	"github.com/rztaylor/froodbase/internal/httpserver"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches: no arguments serves the API; "migrate" applies pending
// migrations; "migrate --status" lists them. Exit codes: 0 ok, 1 runtime
// failure, 2 bad configuration or usage.
func run(args []string) int {
	if len(args) > 0 && args[0] == "migrate" {
		return runMigrate(args[1:])
	}

	flags := flag.NewFlagSet("froodbase-api", flag.ContinueOnError)
	showVersion := flags.Bool("version", false, "print the build version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("froodbase-api", version.String())
		return 0
	}

	cfg, err := config.LoadAPI(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api:", err)
		return 2
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("froodbase-api failed", "err", err)
		return 1
	}
	defer pool.Close()

	if err := httpserver.New(log).Run(ctx, cfg.Addr); err != nil {
		log.Error("froodbase-api failed", "err", err)
		return 1
	}
	return 0
}

func runMigrate(args []string) int {
	flags := flag.NewFlagSet("froodbase-api migrate", flag.ContinueOnError)
	status := flags.Bool("status", false, "list migrations and whether they are applied instead of applying them")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadAPI(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api migrate:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api migrate:", err)
		return 1
	}
	defer pool.Close()

	if *status {
		statuses, err := pool.Status(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "froodbase-api migrate:", err)
			return 1
		}
		for _, s := range statuses {
			state := "pending"
			if s.Applied {
				state = "applied " + s.AppliedAt.UTC().Format("2006-01-02T15:04:05Z")
			}
			fmt.Printf("%5d  %-9s  %s\n", s.Version, state, s.Source)
		}
		return 0
	}

	applied, err := pool.Migrate(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api migrate:", err)
		return 1
	}
	if len(applied) == 0 {
		fmt.Println("migrations: nothing to apply")
		return 0
	}
	fmt.Printf("migrations: applied %d (up to version %d)\n", len(applied), applied[len(applied)-1])
	return 0
}
