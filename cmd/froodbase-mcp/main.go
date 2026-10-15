package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/client"
	"github.com/rztaylor/froodbase/internal/config"
	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run serves MCP over stdio until stdin closes or a signal arrives. Logs go
// to stderr because stdout is the protocol stream. Exit codes: 0 ok, 1
// runtime failure, 2 bad configuration or usage.
func run(args []string) int {
	flags := flag.NewFlagSet("froodbase-mcp", flag.ContinueOnError)
	showVersion := flags.Bool("version", false, "print the build version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("froodbase-mcp", version.String())
		return 0
	}

	cfg, err := config.LoadMCP(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-mcp:", err)
		return 2
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := client.New(cfg.APIURL, cfg.Username, cfg.Password)
	server := mcpserver.New(version.Version, mcpserver.Handlers{
		Whoami: api.Whoami,
	})
	log.Info("froodbase-mcp starting", "version", version.Version, "api_url", cfg.APIURL, "user", cfg.Username)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Error("froodbase-mcp failed", "err", err)
		return 1
	}
	return 0
}
