package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: feat <command>")
		fmt.Fprintln(os.Stderr, "commands: serve, init, version")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println("feat v0.1.0")
	case "init":
		fmt.Println("TODO: init")
	case "serve":
		fmt.Println("TODO: serve")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
