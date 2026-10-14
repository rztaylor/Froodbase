package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rztaylor/froodbase/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("froodbase-mcp", version.String())
		return
	}

	fmt.Fprintln(os.Stderr, "froodbase-mcp: not implemented yet; see docs/planning/04-implementation-plan.md")
	os.Exit(2)
}
