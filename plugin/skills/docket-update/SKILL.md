---
name: docket-update
description: Update the docket CLAUDE.md snippet in the current project to the latest version. Use when the user says /docket-update, wants to update docket config, sync docket instructions, or refresh the CLAUDE.md snippet.
---

# docket-update: Sync CLAUDE.md with latest docket snippet

Update the Feature Tracking (docket) section in the current project's CLAUDE.md to the latest version.

## Steps

1. **Run the update command:**
   ```bash
   ${CLAUDE_PLUGIN_ROOT}/docket.exe update
   ```

2. **Report the result** to the user. The command will either:
   - Update the existing section in place
   - Insert the section after the first heading if not found
   - Report that it's already up to date

3. **If CLAUDE.md doesn't exist**, tell the user to run `/docket-init` first instead.
