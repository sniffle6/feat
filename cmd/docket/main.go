package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/server"

	staticfiles "github.com/sniffle6/claude-docket/dashboard"
	"github.com/sniffle6/claude-docket/internal/dashboard"
	docketmcp "github.com/sniffle6/claude-docket/internal/mcp"
	"github.com/sniffle6/claude-docket/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: docket <command>")
		fmt.Fprintln(os.Stderr, "commands: serve, init, update, hook, version")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println("docket v0.1.0")
	case "init":
		runInit()
	case "hook":
		runHook()
	case "serve":
		runServe()
	case "update":
		runUpdate()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// portForDir returns a stable port in the range 7890-8890 based on the
// absolute project path. Different projects get different ports so multiple
// docket instances can run simultaneously.
func portForDir(dir string) int {
	cleaned := filepath.Clean(strings.ToLower(dir))
	h := sha256.Sum256([]byte(cleaned))
	n := binary.BigEndian.Uint16(h[:2])
	return 7890 + int(n)%1000
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
	fmt.Println("Initialized .docket/ in", dir)
	fmt.Println("Add .docket/ to your .gitignore.")
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

	// Start HTTP dashboard in background on a per-project port
	port := portForDir(dir)

	// Write port file so skills/tools can discover the dashboard URL
	portFile := filepath.Join(dir, ".docket", "port")
	os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)

	go func() {
		handler := dashboard.NewHandler(s, staticfiles.StaticFS, dir)
		addr := fmt.Sprintf(":%d", port)
		log.Printf("Dashboard: http://localhost:%d", port)
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Printf("dashboard error: %v", err)
		}
	}()

	// Run MCP server on stdio (blocks)
	mcpServer := docketmcp.NewServer(s)
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
