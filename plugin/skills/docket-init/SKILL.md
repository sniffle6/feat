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

3. **Check if CLAUDE.md already has a docket section** (grep for "## Feature Tracking (docket)"). If it does, tell the user to run `/docket-update` to refresh it, and stop.

4. **Run the update command** to insert the docket section into CLAUDE.md:
   ```bash
   ${CLAUDE_PLUGIN_ROOT}/docket.exe update
   ```
   This inserts the latest snippet after the first heading in CLAUDE.md.

5. **Add `.docket/` to .gitignore** if it exists and doesn't already have it.

6. **Confirm** to the user that docket is initialized. Mention they can run `/docket` to open the dashboard.

## Notes

- The docket MCP server starts automatically when the plugin is installed. No separate server launch needed.
- If CLAUDE.md has an old "Feature Tracking (feat)" section, the update command will replace it.
- The snippet is embedded in the docket binary, so it's always in sync with the installed version.
