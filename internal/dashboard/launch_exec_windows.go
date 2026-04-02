package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// launchInTerminal opens a new terminal window running claude for the given feature.
func launchInTerminal(projDir, promptPath, featureTitle, featureID, launchDir string) error {
	// Write a .cmd launcher script to avoid nested quoting issues.
	cmdScript := fmt.Sprintf("@echo off\r\ncd /d \"%s\"\r\nclaude --dangerously-skip-permissions --append-system-prompt-file \"%s\" \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\r\n",
		projDir, promptPath, featureTitle, featureID)
	cmdPath := filepath.Join(launchDir, featureID+".cmd")
	if err := os.WriteFile(cmdPath, []byte(cmdScript), 0644); err != nil {
		return fmt.Errorf("failed to write launch script: %w", err)
	}

	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`cmd /C start "" wt cmd /k "%s"`, cmdPath),
	}
	cmd.Dir = projDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch: %w", err)
	}
	go cmd.Wait()
	return nil
}
