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
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("froodbase-api", version.String())
		return
	}

	cfg, err := config.LoadAPI(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api:", err)
		os.Exit(2)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := httpserver.New(log).Run(ctx, cfg.Addr); err != nil {
		log.Error("froodbase-api failed", "err", err)
		os.Exit(1)
	}
}
