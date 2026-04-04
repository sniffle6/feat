package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

func bindSessionHandler(s *store.Store, binding *Binding, launchFeature string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID, _ := args["feature_id"].(string)
		sessionID, _ := args["session_id"].(string)
		if featureID == "" || sessionID == "" {
			return mcp.NewToolResultError("feature_id and session_id are required"), nil
		}

		// Already bound to this session? Return early.
		if _, cachedFeature, cachedSession, ok := binding.Get(); ok && cachedSession == sessionID {
			return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q (already bound).", cachedFeature)), nil
		}

		// Try to find existing open session for this Claude session
		ws, err := s.GetWorkSessionByClaudeSession(sessionID)
		if err != nil {
			// No existing session — open a new one
			ws, err = s.OpenWorkSession(featureID, sessionID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to open work session: %v", err)), nil
			}
		}

		// Claim with mcp_pid
		pid := int64(os.Getpid())
		s.SetMcpPid(ws.ID, &pid)

		// Cache identity
		binding.Set(ws.ID, ws.FeatureID, ws.ClaudeSessionID)

		return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q, work session #%d.", ws.FeatureID, ws.ID)), nil
	}
}
