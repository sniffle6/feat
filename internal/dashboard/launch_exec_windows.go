package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// isWindowAlive checks if a terminal window with a title containing "docket-<featureID>"
// exists. Uses PowerShell to search process window titles.
func isWindowAlive(featureID string) bool {
	ps := fmt.Sprintf(
		`if(Get-Process|Where-Object{$_.MainWindowTitle -like '*docket-%s*' -and $_.MainWindowHandle -ne 0}|Select-Object -First 1){exit 0}else{exit 1}`,
		featureID,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	return cmd.Run() == nil
}

// launchInTerminal opens a new terminal window running claude for the given feature.
func launchInTerminal(projDir, promptPath, featureTitle, featureID, launchDir string) error {
	// Write a .cmd launcher script. Sets window title for process discovery by isWindowAlive.
	cmdScript := fmt.Sprintf("@echo off\r\ntitle docket-%s\r\ncd /d \"%s\"\r\nclaude --dangerously-skip-permissions --append-system-prompt-file \"%s\" \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\r\n",
		featureID, projDir, promptPath, featureTitle, featureID)
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
	} else if _, err := exec.LookPath("wt"); err == nil {
		// Default: Windows Terminal with named window (no start wrapper needed)
		tmpl := `wt -w docket-{{feature_id}} --title {{feature_title}} cmd /k {{script_path}}`
		cmdLine = "cmd /C " + SubstituteTemplate(tmpl, vars, "windows")
	} else {
		// Fallback: no wt — use start to open in a new window
		tmpl := `cmd /c start {{feature_title}} cmd /k {{script_path}}`
		cmdLine = SubstituteTemplate(tmpl, vars, "windows")
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
func focusTerminal(projDir, featureID, featureTitle string) error {
	cfg := ReadLaunchConfig(projDir)
	if cfg.Focus == "" {
		return fmt.Errorf("no focus command configured in launch.toml")
	}

	vars := TemplateVars{
		FeatureID:    featureID,
		FeatureTitle: featureTitle,
		ProjectDir:   projDir,
	}

	cmdLine := "cmd /C " + SubstituteTemplate(cfg.Focus, vars, "windows")
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: cmdLine,
	}
	cmd.Dir = projDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("focus command failed: %w", err)
	}
	return nil
}
