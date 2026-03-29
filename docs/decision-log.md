# Decision Log

Structured decision tracking per feature. Prevents Claude from re-exploring dead ends across sessions.

## What It Does

Each feature can have decisions logged against it. A decision records:
- **Approach**: what was considered (e.g., "Use websockets for real-time updates")
- **Outcome**: `accepted` or `rejected`
- **Reason**: why (e.g., "Too complex for MVP, polling sufficient")

## How It Works

- `add_decision` MCP tool logs a decision on a feature
- `get_context` shows rejected decisions in the briefing so Claude avoids dead ends
- `get_feature` and `get_full_context` include all decisions (accepted + rejected)
- Dashboard shows decisions in the feature detail panel

## Key Files

- `internal/store/decision.go` — Decision struct, AddDecision, GetDecisionsForFeature
- `internal/store/migrate.go` — schemaV5 with decisions table
- `internal/mcp/tools.go` — add_decision tool handler
- `dashboard/index.html` — decisions section in feature detail panel
