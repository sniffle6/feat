package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// isWindowAlive checks if the terminal launched for a feature is still running.
// The .cmd script writes its PID (via a small PowerShell one-liner) to a .pid file.
// We check if that PID is still an active process.
func isWindowAlive(projDir, featureID string) bool {
	pidPath := filepath.Join(projDir, ".docket", "launch", featureID+".pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false // no PID file → not alive
	}

	pid := string(data)
	// Trim any whitespace/newlines
	for len(pid) > 0 && (pid[len(pid)-1] == '\n' || pid[len(pid)-1] == '\r' || pid[len(pid)-1] == ' ') {
		pid = pid[:len(pid)-1]
	}
	if pid == "" {
		return false
	}

	// Check if process exists — tasklist exits 0 if found
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`cmd /C tasklist /FI "PID eq %s" /NH 2>nul | findstr /C:"%s" >nul`, pid, pid),
	}
	return cmd.Run() == nil
}

// launchInTerminal opens a new terminal window running claude for the given feature.
func launchInTerminal(projDir, promptPath, featureTitle, featureID, launchDir string) error {
	// Write a .cmd launcher script. Writes the cmd.exe PID to a .pid file so
	// isWindowAlive can check if the terminal is still running.
	// PowerShell's $PID parent is the cmd.exe running this script.
	pidPath := filepath.Join(launchDir, featureID+".pid")
	cmdScript := fmt.Sprintf("@echo off\r\ntitle docket-%s\r\npowershell -NoProfile -Command \"(Get-CimInstance Win32_Process -Filter ('ProcessId='+$PID)).ParentProcessId | Out-File -Encoding ascii -NoNewline '%s'\"\r\ncd /d \"%s\"\r\nclaude --dangerously-skip-permissions --append-system-prompt-file \"%s\" \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\r\ndel \"%s\" 2>nul\r\n",
		featureID, pidPath, projDir, promptPath, featureTitle, featureID, pidPath)
	cmdPath := filepath.Join(launchDir, featureID+".cmd")
	if err := os.WriteFile(cmdPath, []byte(cmdScript), 0644); err != nil {
		return fmt.Errorf("failed to write launch script: %w", err)
	}

	vars := TemplateVars{
		FeatureID:    featureID,
		FeatureTitle: featureTitle,
		ScriptPath:   cmdPath,
		ProjectDir:   projDir,
	}

	cfg := ReadLaunchConfig(projDir)

	var cmdLine string
	if cfg.Launch != "" {
		cmdLine = "cmd /C " + SubstituteTemplate(cfg.Launch, vars, "windows")
	} else {
		// Default: open a new console window via "start". Each session gets its
		// own window with its own process — SetForegroundWindow can target them
		// individually. The window title is set by the .cmd script ("title docket-<id>").
		// We avoid wt named windows because WT runs as a single process with
		// tabs, making individual window focus impossible.
		cmdLine = fmt.Sprintf(`cmd /C start "docket-%s" cmd /k %s`, featureID, ShellEscape(cmdPath, "windows"))
	}

	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: cmdLine,
	}
	cmd.Dir = projDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch: %w", err)
	}
	go cmd.Wait()
	return nil
}

// focusTerminal brings an existing terminal window for the given feature into focus.
// If a focus command is configured in launch.toml, uses that.
// Otherwise uses the PID file to find the process and walks up to its
// window-owning ancestor, then calls SetForegroundWindow via PowerShell.
func focusTerminal(projDir, featureID, featureTitle string) error {
	cfg := ReadLaunchConfig(projDir)

	if cfg.Focus != "" {
		// User-configured focus command
		vars := TemplateVars{
			FeatureID:    featureID,
			FeatureTitle: featureTitle,
			ProjectDir:   projDir,
		}
		cmdLine := "cmd /C " + SubstituteTemplate(cfg.Focus, vars, "windows")
		cmd := exec.Command("cmd")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdLine}
		cmd.Dir = projDir
		return cmd.Run()
	}

	// Default: use PID file → find ancestor window → SetForegroundWindow
	pidPath := filepath.Join(projDir, ".docket", "launch", featureID+".pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("no PID file for feature %s", featureID)
	}
	pid := string(data)
	for len(pid) > 0 && (pid[len(pid)-1] == '\n' || pid[len(pid)-1] == '\r' || pid[len(pid)-1] == ' ') {
		pid = pid[:len(pid)-1]
	}

	// PowerShell: walk up the process tree from the cmd.exe PID to find the
	// first ancestor with a MainWindowHandle, then bring it to the foreground.
	ps := fmt.Sprintf(`
Add-Type -Name W -Namespace W -MemberDefinition '[DllImport("user32.dll")]public static extern bool SetForegroundWindow(IntPtr h);[DllImport("user32.dll")]public static extern bool ShowWindow(IntPtr h,int c);[DllImport("user32.dll")]public static extern bool IsIconic(IntPtr h);'
$cpid = %s
for ($i = 0; $i -lt 10; $i++) {
    $p = Get-Process -Id $cpid -ErrorAction SilentlyContinue
    if ($p -and $p.MainWindowHandle -ne 0) {
        if ([W.W]::IsIconic($p.MainWindowHandle)) { [W.W]::ShowWindow($p.MainWindowHandle, 9) | Out-Null }
        [W.W]::SetForegroundWindow($p.MainWindowHandle) | Out-Null
        exit 0
    }
    $parent = (Get-CimInstance Win32_Process -Filter "ProcessId=$cpid" -ErrorAction SilentlyContinue).ParentProcessId
    if (-not $parent -or $parent -eq $cpid) { break }
    $cpid = $parent
}
exit 1
`, pid)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.Dir = projDir
	return cmd.Run()
}
