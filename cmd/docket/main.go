package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"

	staticfiles "github.com/sniffle6/claude-docket/dashboard"
	"github.com/sniffle6/claude-docket/internal/checkpoint"
	"github.com/sniffle6/claude-docket/internal/dashboard"
	docketmcp "github.com/sniffle6/claude-docket/internal/mcp"
	"github.com/sniffle6/claude-docket/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: docket <command>")
		fmt.Fprintln(os.Stderr, "commands: serve, init, update, export, hook, version")
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
	case "export":
		runExport()
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

	// Dashboard leader election — try to bind port, fall back to standby polling
	port := portForDir(dir)
	portStr := fmt.Sprintf("%d", port)

	// Write port file (all instances write the same deterministic port)
	portFile := filepath.Join(dir, ".docket", "port")
	os.WriteFile(portFile, []byte(portStr), 0644)

	handler := dashboard.NewHandler(s, staticfiles.StaticFS, dir)

	tryServeDashboard := func() bool {
		ln, err := net.Listen("tcp", ":"+portStr)
		if err != nil {
			return false // port taken — another instance is the leader
		}
		go func() {
			log.Printf("Dashboard: http://localhost:%s", portStr)
			if err := http.Serve(ln, handler); err != nil {
				log.Printf("dashboard serve error: %v", err)
			}
		}()
		return true
	}

	// Try to become leader immediately
	if !tryServeDashboard() {
		// Standby — probe every 3 seconds
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if tryServeDashboard() {
					return
				}
			}
		}()
	}

	// Start checkpoint worker in background
	cfg := checkpoint.LoadConfig()
	var summarizer checkpoint.SummarizerBackend
	if cfg.Enabled {
		summarizer = checkpoint.NewAnthropicSummarizer(cfg)
		log.Printf("Checkpoint summarizer: enabled (model: %s)", cfg.Model)
	} else {
		summarizer = &checkpoint.NoopSummarizer{}
		log.Printf("Checkpoint summarizer: disabled (no ANTHROPIC_API_KEY)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := checkpoint.NewWorker(s, summarizer)
	go worker.Run(ctx, 500*time.Millisecond)

	// Run MCP server on stdio (blocks)
	mcpServer := docketmcp.NewServer(s, dir, worker.Notify)
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
