---
name: docket
description: Open the docket dashboard in the default browser. Use when the user says /docket, mentions the docket dashboard, or wants to check feature tracking status.
---

# docket: Open Dashboard

Open the docket feature-tracking dashboard in the default browser.

## Steps

1. **Read the port file** to get the dashboard port:
   ```bash
   cat .docket/port
   ```
   If the file doesn't exist, fall back to port `7890`.

2. **Open the dashboard** using the discovered port. Detect the platform and use the appropriate command:
   - **Windows**: `start http://localhost:<port>`
   - **macOS**: `open http://localhost:<port>`
   - **Linux**: `xdg-open http://localhost:<port>`

3. **Confirm** to the user that the dashboard is opening, and mention the URL.

## Notes

- The docket MCP server must be running for the dashboard to load. If the user reports a blank page, check that the docket server is registered in `.mcp.json` and active.
- Each project gets its own port (derived from the project path), so multiple projects can run simultaneously.
- For quick status without leaving the terminal, the main session can call `mcp__plugin_docket_docket__list_features` or `mcp__plugin_docket_docket__get_ready` directly.
