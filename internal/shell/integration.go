package shell

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/termia/termia/embedded"
	"github.com/termia/termia/internal/config"
)

// GetIntegrationScript returns the embedded integration script for the given shell type
func GetIntegrationScript(shellType ShellType) (string, error) {
	switch shellType {
	case ShellZsh:
		return embedded.TermiaZsh, nil
	case ShellBash:
		return embedded.TermiaBash, nil
	case ShellFish:
		return "", fmt.Errorf("fish shell integration not yet supported")
	default:
		return "", fmt.Errorf("unsupported shell type: %s", shellType)
	}
}

// GetShimScript returns the embedded shim rc file for the given shell type.
func GetShimScript(shellType ShellType) (string, error) {
	switch shellType {
	case ShellZsh:
		return embedded.TermiaZshRC, nil
	case ShellBash:
		return embedded.TermiaBashRC, nil
	case ShellFish:
		return "", fmt.Errorf("fish shell integration not yet supported")
	default:
		return "", fmt.Errorf("unsupported shell type: %s", shellType)
	}
}

// InstallIntegration installs the shell integration script to the config directory
func InstallIntegration(shellType ShellType) error {
	// Get script content
	script, err := GetIntegrationScript(shellType)
	if err != nil {
		return err
	}

	shim, err := GetShimScript(shellType)
	if err != nil {
		return err
	}

	// Determine filenames
	var integrationFile string
	var shimFile string
	switch shellType {
	case ShellZsh:
		integrationFile = "termia.zsh"
		shimFile = ".zshrc"
	case ShellBash:
		integrationFile = "termia.bash"
		shimFile = "termia.bashrc"
	default:
		return fmt.Errorf("unsupported shell type: %s", shellType)
	}

	// Get shell directory from config
	shellDir := config.ShellDir()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(shellDir, 0755); err != nil {
		return fmt.Errorf("failed to create shell directory: %w", err)
	}

	// Write scripts to file
	integrationPath := filepath.Join(shellDir, integrationFile)
	integrationContent := []byte(script)
	if existing, err := os.ReadFile(integrationPath); err == nil {
		if !bytes.Equal(existing, integrationContent) {
			if err := os.WriteFile(integrationPath, integrationContent, 0755); err != nil {
				return fmt.Errorf("failed to write integration script: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read integration script: %w", err)
	} else if err := os.WriteFile(integrationPath, integrationContent, 0755); err != nil {
		return fmt.Errorf("failed to write integration script: %w", err)
	}

	shimPath := filepath.Join(shellDir, shimFile)
	shimContent := []byte(shim)
	if existing, err := os.ReadFile(shimPath); err == nil {
		if !bytes.Equal(existing, shimContent) {
			if err := os.WriteFile(shimPath, shimContent, 0755); err != nil {
				return fmt.Errorf("failed to write shim script: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read shim script: %w", err)
	} else if err := os.WriteFile(shimPath, shimContent, 0755); err != nil {
		return fmt.Errorf("failed to write shim script: %w", err)
	}

	return nil
}

// PrintInitScript returns the eval-able snippet that users add to their RC file
func PrintInitScript(shellType ShellType) (string, error) {
	shellDir := config.ShellDir()

	switch shellType {
	case ShellZsh:
		scriptPath := filepath.Join(shellDir, "termia.zsh")
		return fmt.Sprintf("source \"%s\"", scriptPath), nil
	case ShellBash:
		scriptPath := filepath.Join(shellDir, "termia.bash")
		return fmt.Sprintf("source \"%s\"", scriptPath), nil
	case ShellFish:
		return "", fmt.Errorf("fish shell integration not yet supported")
	default:
		return "", fmt.Errorf("unsupported shell type: %s", shellType)
	}
}

// GetShimCommand returns the command args to launch shell with integration
func GetShimCommand(shellType ShellType, shellPath string) []string {
	switch shellType {
	case ShellZsh:
		// For zsh, we use ZDOTDIR trick
		// Create temp dir with .zshrc that sources user's then termia's
		// This is handled by the caller, so we just return the shell path
		// The caller will set up ZDOTDIR environment variable
		return []string{shellPath}

	case ShellBash:
		// For bash, use --rcfile to load shim
		shellDir := config.ShellDir()
		rcFile := filepath.Join(shellDir, "termia.bashrc")
		return []string{shellPath, "--rcfile", rcFile}

	default:
		// Fallback: just launch the shell
		return []string{shellPath}
	}
}
