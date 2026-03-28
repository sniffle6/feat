---
name: docket-init
description: Initialize docket feature tracking in the current project. Use when the user says /docket-init, wants to set up docket, add feature tracking, or initialize docket in a new project.
---

# docket-init: Set up feature tracking

Initialize docket in the current project. Creates the database directory and adds dispatch instructions to CLAUDE.md.

## Steps

1. **Create the database directory** if it doesn't exist:
   ```bash
   mkdir -p .docket
   ```

2. **Check if CLAUDE.md exists** at the project root. If not, create it with a project heading.

3. **Check if CLAUDE.md already has a docket section** (grep for "Feature Tracking (docket)"). If it does, tell the user it's already set up and stop.

4. **Append the docket section** to the end of CLAUDE.md:

   ```markdown

   ## Feature Tracking (docket)

   This project uses `docket` for feature tracking. Dashboard: http://localhost:7890 (or run `/docket`).

   Dispatch the `board-manager` agent (model: sonnet) at these points:
   1. **Start of implementation work** — skip for questions/reviews/lookups
   2. **After a commit** — pass commit hash, message, files, feature ID
   3. **Session ending** — pass summary, commits, files, feature ID

   Carry the feature ID the agent returns across dispatches. `get_ready` stays in main session.
   ```

5. **Add `.docket/` to .gitignore** if it exists and doesn't already have it.

6. **Confirm** to the user that docket is initialized. Mention they can run `/docket` to open the dashboard.

## Notes

- The docket MCP server starts automatically when the plugin is installed. No separate server launch needed.
- If CLAUDE.md has an old "Feature Tracking (feat)" section, replace it with the docket version.
