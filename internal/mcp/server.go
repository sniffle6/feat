package mcp

import (
	"os"
	"sync"

	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

// Binding holds the cached session identity for this MCP server instance.
type Binding struct {
	mu              sync.RWMutex
	WorkSessionID   int64
	FeatureID       string
	ClaudeSessionID string
	Bound           bool
}

func (b *Binding) Get() (wsID int64, featureID, claudeSessionID string, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.WorkSessionID, b.FeatureID, b.ClaudeSessionID, b.Bound
}

func (b *Binding) Set(wsID int64, featureID, claudeSessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WorkSessionID = wsID
	b.FeatureID = featureID
	b.ClaudeSessionID = claudeSessionID
	b.Bound = true
}

func (b *Binding) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WorkSessionID = 0
	b.FeatureID = ""
	b.ClaudeSessionID = ""
	b.Bound = false
}

func NewServer(s *store.Store, projectDir string, onCheckpoint func()) *server.MCPServer {
	srv := server.NewMCPServer("docket", "0.1.0",
		server.WithToolCapabilities(true),
	)

	binding := &Binding{}

	// Auto-bind for dashboard launches via env var
	launchFeature := os.Getenv("DOCKET_LAUNCH_FEATURE")

	registerTools(srv, s, projectDir, onCheckpoint, binding, launchFeature)
	return srv
}
