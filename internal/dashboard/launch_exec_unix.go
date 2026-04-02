//go:build !windows

package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// launchInTerminal opens a new terminal window running claude for the given feature.
func launchInTerminal(projDir, promptPath, featureTitle, featureID, launchDir string) error {
	// Write a shell launcher script
	script := fmt.Sprintf("#!/bin/sh\ncd %q\nclaude --dangerously-skip-permissions --append-system-prompt-file %q \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\n",
		projDir, promptPath, featureTitle, featureID)
	scriptPath := filepath.Join(launchDir, featureID+".sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write launch script: %w", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// macOS: open a new Terminal.app window
		cmd = exec.Command("open", "-a", "Terminal", scriptPath)
	default:
		// Linux: try common terminal emulators
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "xterm"} {
			if path, err := exec.LookPath(term); err == nil {
				if term == "gnome-terminal" {
					cmd = exec.Command(path, "--", scriptPath)
				} else {
					cmd = exec.Command(path, "-e", scriptPath)
				}
				break
			}
		}
		if cmd == nil {
			// Fallback: run in background without a terminal
			cmd = exec.Command("sh", scriptPath)
		}
	}

	cmd.Dir = projDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch: %w", err)
	}
	go cmd.Wait()
	return nil
}
