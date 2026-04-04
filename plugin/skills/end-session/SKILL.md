---
name: end-session
description: End the current Docket work session without closing Claude. Forces a final checkpoint, writes the handoff file, and closes the work session. Use when switching features or finishing work but keeping Claude open.
---

# end-session: End Docket Work Session

End the current Docket work session and write the handoff file.

## Steps

1. **Ask the user** if they want to set a `left_off` note before closing:
   > "Want to set a 'left off' note for the next session? (optional)"

2. **If they provide a note**, call `mcp__plugin_docket_docket__update_feature` with `left_off`.

3. **Close the work session with a final checkpoint:**
   Call `mcp__plugin_docket_docket__checkpoint` with `end_session=true`.
   This forces a checkpoint, writes the handoff file, and closes the work session in one call.

4. **Report** that the work session has been closed and the handoff file has been written.

## Notes

- This does NOT close Claude — only the Docket work session.
- The handoff file will be available at `.docket/handoff/<feature-id>.md`.
- After closing, call `bind_session(feature_id="...", session_id="...")` to start tracking a new feature. The session ID is shown in the session context.
- This is a user-initiated action — Claude should not call this autonomously.
