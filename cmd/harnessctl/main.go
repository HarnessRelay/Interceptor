package main

import (
	"fmt"
	"os"
)

const version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage()
		return
	}

	switch args[0] {
	case "version":
		fmt.Println(version)
	case "run", "attach", "list", "stop":
		fmt.Fprintf(os.Stderr, "harnessctl %s is not implemented yet\n", args[0])
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`harnessctl controls a local harnessd daemon.

Usage:
  harnessctl --help
  harnessctl version
  harnessctl run <command> [args...]   (not implemented)
  harnessctl attach <session-id>       (not implemented)
  harnessctl list                      (not implemented)
  harnessctl stop <session-id>         (not implemented)
`)
}
