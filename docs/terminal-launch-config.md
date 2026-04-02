# Terminal Launch Configuration

Docket's dashboard has a play button on each feature card that launches a Claude session. By default it uses Windows Terminal (with named windows) on Windows, Terminal.app on macOS, and auto-detected terminals on Linux.

## Custom Terminal

Create a `launch.toml` file with your terminal's commands:

**Global (all projects):** `~/.config/docket/launch.toml` (Unix) or `%APPDATA%/docket/launch.toml` (Windows)

**Per-project (overrides global):** `.docket/launch.toml`

Format:

```toml
launch = "wt -w docket-{{feature_id}} --title {{feature_title}} cmd /k {{script_path}}"
focus = "wt -w docket-{{feature_id}}"
```

### Template Variables

- `{{feature_id}}` — docket feature slug (never escaped, safe in flags)
- `{{feature_title}}` — human-readable title (auto-escaped for your shell)
- `{{script_path}}` — path to the generated launcher script (auto-escaped)
- `{{project_dir}}` — project root directory (auto-escaped)

Do NOT manually quote template variables — escaping is automatic.

### Focus Command

The `focus` key is optional. When set, clicking the play button on a card with an active session runs the focus command to bring its terminal window forward. If focus fails (window was closed), the dashboard offers "Close & Relaunch".

Without a focus command, the dashboard shows the window name for manual alt-tab.

## Examples

See `examples/launch-*.toml` for configs for Windows Terminal, iTerm2, Kitty, tmux, and Alacritty.

## How It Works

- Commands run via `cmd /C` (Windows) or `sh -c` (Unix)
- The `launch` command opens a terminal running a generated script (`.cmd`/`.sh`) that starts Claude
- Script generation is unchanged — `launch.toml` controls how the terminal opens, not what runs inside it

## Key Files

- `internal/dashboard/launch.go` — config reader, template substitution, shell escaping
- `internal/dashboard/launch_exec_windows.go` — Windows launcher and focus
- `internal/dashboard/launch_exec_unix.go` — Unix launcher and focus
- `internal/dashboard/dashboard.go` — `/api/launch/{id}` and `/api/session/{id}` endpoints
- `dashboard/index.html` — frontend button behavior
- `examples/launch-*.toml` — example configs
