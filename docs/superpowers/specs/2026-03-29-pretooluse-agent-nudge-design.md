# PreToolUse Agent Nudge — Design Spec

## Problem

When superpowers' plan execution skills (executing-plans, subagent-driven-development) dispatch subagents, they follow their own prescriptive workflow (TodoWrite-based tracking) and ignore the CLAUDE.md docket instructions. This results in:

1. Docket feature cards never getting task items added from the plan
2. No `get_ready` call before work starts
3. Session ends with docket having no record of what was planned or completed

The CLAUDE.md snippet already says "Start of work... call get_ready" but this loses to superpowers' skill instructions which are more prescriptive.

## Solution

A PreToolUse hook on the `Agent` tool that checks docket state before subagent dispatch. Nudges once if tracking isn't set up, then stays silent. Combined with a conditional CLAUDE.md snippet paragraph for superpowers users.

## Design

### 1. PreToolUse Hook (hook.go)

**New handler: `handlePreToolUse`**

Fires when `tool_name == "Agent"`. Logic:

1. Check `.docket/` exists — silent pass-through if not a docket project
2. Check sentinel file `.docket/agent-nudged` — silent pass-through if already nudged this session
3. Open store, call `ListFeatures("in_progress")`
4. Branch:
   - **No in_progress features** — write sentinel, return systemMessage: `[docket] No active docket feature. Call get_ready to set up tracking before dispatching subagents.`
   - **Feature exists, check task items** — call `GetSubtasksForFeature(feature.ID, false)`, count total items across all subtasks
     - **Zero task items** — write sentinel, return systemMessage: `[docket] Active feature "<title>" (id: <id>) has no task items. Add task items from the plan before dispatching subagents.`
     - **Has task items** — silent pass-through (no sentinel write needed, feature is properly set up)

**Output struct** (new, PreToolUse-specific):

```go
type preToolUseOutput struct {
    HookSpecificOutput *preToolUseDecision `json:"hookSpecificOutput,omitempty"`
    SystemMessage      string              `json:"systemMessage,omitempty"`
}

type preToolUseDecision struct {
    PermissionDecision string `json:"permissionDecision"`
}
```

Always returns `permissionDecision: "allow"` — never blocks the dispatch. The nudge is informational only.

**Sentinel file:** `.docket/agent-nudged`
- Written on first nudge (empty file, just needs to exist)
- Checked before nudging — if exists, skip entirely
- Cleared in `handleSessionStart` alongside `commits.log`

### 2. hooks.json Update

Add PreToolUse matcher:

```json
"PreToolUse": [
  {
    "matcher": "Agent",
    "hooks": [
      {
        "type": "command",
        "command": "DOCKET_EXE_PATH hook",
        "timeout": 5
      }
    ]
  }
]
```

Same pattern as existing entries — binary path placeholder replaced by install.sh.

### 3. SessionStart Cleanup (hook.go)

In `handleSessionStart`, add after the `commits.log` clear:

```go
// Clear agent-nudged sentinel
os.Remove(filepath.Join(h.CWD, ".docket", "agent-nudged"))
```

### 4. Conditional Snippet in update.go

**Superpowers detection:** Check if `~/.claude/plugins/installed_plugins.json` contains `"superpowers"`. Use `os.UserHomeDir()` + read + string contains check. Simple and reliable.

**When superpowers is detected**, append this paragraph to `docketSection` before the "After a commit" section:

```
**Plan execution (superpowers):** When using executing-plans or subagent-driven-development,
set up docket BEFORE dispatching the first task — call `get_ready`, create/find a feature card,
and use `add_task_item` for each plan task. A PreToolUse hook will remind you if you forget.
```

**When superpowers is NOT detected**, omit the paragraph. Keeps the snippet clean for non-superpowers users.

**Implementation:** Split `docketSection` into `docketSectionBase` + `docketSectionSuperpowers` + `docketSectionTail`. The `runUpdate` function assembles them based on detection.

## Files Changed

| File | Change |
|------|--------|
| `cmd/docket/hook.go` | Add `handlePreToolUse`, new output structs, sentinel write/check, clear sentinel in SessionStart |
| `plugin/hooks/hooks.json` | Add PreToolUse matcher for Agent |
| `cmd/docket/update.go` | Split snippet const, superpowers detection, conditional assembly |

## Edge Cases

- **No `.docket/` directory** — hook returns silent pass-through. Same as all other hooks.
- **quick_track features** — These have no subtasks by design, but quick_track workflows don't dispatch Agent, so the nudge won't fire.
- **User intentionally skips docket** — Sentinel prevents repeated nagging. One nudge per session, then silent.
- **Multiple in_progress features** — Check the first (most recent). Same pattern as Stop hook.
- **Plan auto-imported but no manual task items** — Auto-imported plans create subtasks/items via `ImportPlan`. If the plan was committed and auto-imported before Agent dispatch, the hook will see the items and stay silent. This is the happy path.
- **Subagent dispatches from within subagents** — PreToolUse fires for each Agent call in the main session. Subagent Agent calls don't trigger plugin hooks (they run in isolated context).

## Testing

- Store tests: existing `GetSubtasksForFeature` coverage is sufficient
- Hook tests: test `handlePreToolUse` with mock hookInput, verify output for each branch (no features, features without items, features with items, sentinel exists)
- Integration: manual test with superpowers executing-plans — verify nudge fires before first Agent dispatch, doesn't fire after task items added
