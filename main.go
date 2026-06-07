package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "status":
		fmt.Println("status: not implemented yet")
	case "sync":
		fmt.Println("sync: not implemented yet")
	default:
		fmt.Fprintf(os.Stderr, "replct: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `replct - one-way disk replicator

Usage:
  replct <command>

Commands:
  status   compare source and replica, report divergence (read-only)
  sync     replicate changes from source to replica
`)
}
