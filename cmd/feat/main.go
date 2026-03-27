package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/server"

	featmcp "github.com/sniffyanimal/feat/internal/mcp"
	"github.com/sniffyanimal/feat/internal/store"
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
		runInit()
	case "serve":
		runServe()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runInit() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}
	s, err := store.Open(dir)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	s.Close()
	fmt.Println("Initialized .feat/ in", dir)
	fmt.Println("Add .feat/ to your .gitignore.")
}

func runServe() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}
	s, err := store.Open(dir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	// Start HTTP dashboard in background (placeholder for now, Task 8 adds real handler)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("feat dashboard — coming soon"))
		})
		log.Printf("Dashboard: http://localhost:7890")
		if err := http.ListenAndServe(":7890", mux); err != nil {
			log.Printf("dashboard error: %v", err)
		}
	}()

	// Run MCP server on stdio (blocks)
	mcpServer := featmcp.NewServer(s)
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
