---
name: feat
description: Open the feat dashboard in the default browser. Use when the user says /feat, mentions the feat dashboard, or wants to check feature tracking status.
---

# feat: Open Dashboard

Open the feat feature-tracking dashboard in the default browser.

## Steps

1. **Open the dashboard:**
   ```bash
   start http://localhost:7890
   ```
   This uses the Windows `start` command. On Linux, use `xdg-open http://localhost:7890` instead.

2. **Confirm** to the user that the dashboard is opening.

## Notes

- The feat MCP server must be running for the dashboard to load. If the user reports a blank page, check that the feat server is registered in `.mcp.json` and active.
- For quick status without leaving the terminal, the main session can call `mcp__plugin_feat_feat__list_features` or `mcp__plugin_feat_feat__get_ready` directly.
