package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rztaylor/froodbase/internal/authz/staticusers"
	"github.com/rztaylor/froodbase/internal/config"
	"github.com/rztaylor/froodbase/internal/httpserver"
	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches: no arguments serves the API; "migrate" applies pending
// migrations ("migrate --status" lists them); "hash-password" reads a
// password from stdin and prints its bcrypt hash. Exit codes: 0 ok, 1
// runtime failure, 2 bad configuration or usage.
func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			return runMigrate(args[1:])
		case "hash-password":
			return runHashPassword()
		}
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
	users, err := staticusers.Load(cfg.UsersFile)
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

	mcpServer := mcpserver.New(version.Version, mcpserver.APIHandlers())
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: log},
	)
	srv := httpserver.New(log)
	srv.Mount(httpserver.MCPPath, httpserver.BasicAuth(users, users, mcpHandler))
	log.Info("froodbase-api users loaded", "count", users.Len(), "file", cfg.UsersFile)

	if err := srv.Run(ctx, cfg.Addr); err != nil {
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

	cfg, err := config.LoadMigrate(os.LookupEnv)
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

// runHashPassword reads one line from stdin so the password never appears in
// a shell history or process list, and prints the bcrypt hash.
func runHashPassword() int {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(os.Stderr, "froodbase-api hash-password: read a password from stdin, for example: echo -n 'pw' | froodbase-api hash-password")
		return 2
	}
	hash, err := staticusers.HashPassword(strings.TrimRight(line, "\r\n"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "froodbase-api hash-password:", err)
		return 2
	}
	fmt.Println(hash)
	return 0
}
