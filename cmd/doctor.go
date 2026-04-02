package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/llm"
	"github.com/termia/termia/internal/shell"
)

var doctorCmd *cobra.Command

func init() {
	doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Check Termia installation health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}

	rootCmd.AddCommand(doctorCmd)
}

func runDoctor() error {
	fmt.Println("Termia Doctor")
	fmt.Println("=============")

	passed := 0
	total := 0

	// Check 1: Config file exists
	total++
	configPath := config.ConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("[OK] Config file exists")
		passed++
	} else {
		fmt.Printf("[FAIL] Config file not found at %s\n", configPath)
	}

	// Check 2: Config file parseable
	total++
	if _, err := config.Load(configPath); err == nil {
		fmt.Println("[OK] Config file parseable")
		passed++
	} else {
		fmt.Printf("[FAIL] Config file not parseable: %v\n", err)
	}

	// Check 3: DB directory exists and writable
	total++
	dbPath := config.DBPath()
	dbDir := filepath.Dir(dbPath)
	if info, err := os.Stat(dbDir); err == nil && info.IsDir() {
		// Try to create a test file
		testFile := filepath.Join(dbDir, ".termia_write_test")
		if f, err := os.Create(testFile); err == nil {
			f.Close()
			os.Remove(testFile)
			fmt.Println("[OK] DB directory exists and writable")
			passed++
		} else {
			fmt.Printf("[FAIL] DB directory not writable: %v\n", err)
		}
	} else {
		fmt.Printf("[FAIL] DB directory doesn't exist: %s\n", dbDir)
	}

	// Check 4: DB file exists and can open
	total++
	if _, err := os.Stat(dbPath); err == nil {
		database, err := db.Open(dbPath, logger)
		if err == nil {
			database.Close()
			fmt.Println("[OK] DB file exists and can be opened")
			passed++
		} else {
			fmt.Printf("[FAIL] DB file cannot be opened: %v\n", err)
		}
	} else {
		fmt.Println("[OK] DB file doesn't exist yet (will be created on first use)")
		passed++
	}

	// Check 5: Shell detected
	total++
	shellInfo := shell.Detect()
	if shellInfo.Type != shell.ShellUnknown {
		fmt.Printf("[OK] Shell detected: %s (%s)\n", shellInfo.Type, shellInfo.Path)
		passed++
	} else {
		fmt.Println("[FAIL] Shell not detected")
	}

	// Check 6: Shell integration script installed
	total++
	shellDir := config.ShellDir()
	var scriptName string
	switch shellInfo.Type {
	case shell.ShellZsh:
		scriptName = "termia.zsh"
	case shell.ShellBash:
		scriptName = "termia.bash"
	default:
		scriptName = "termia.zsh" // default check
	}
	scriptPath := filepath.Join(shellDir, scriptName)
	if _, err := os.Stat(scriptPath); err == nil {
		fmt.Println("[OK] Shell integration script installed")
		passed++
	} else {
		fmt.Printf("[FAIL] Shell integration script not found at %s\n", scriptPath)
	}

	// Check 7: LLM API key configured
	total++
	provider := cfg.LLM.DefaultProvider
	providerCfg, ok := cfg.LLM.ProviderConfig(provider)
	if !ok {
		fmt.Printf("[FAIL] Unsupported LLM provider configured: %s\n", provider)
	} else if err := llm.ValidateProviderConfig(cfg.LLM.ProviderKind(provider), providerCfg); err == nil {
		fmt.Printf("[OK] LLM provider configured (%s)\n", cfg.LLM.ProviderDisplayName(provider))
		passed++
	} else {
		fmt.Printf("[FAIL] LLM provider configuration invalid (%s): %v\n", cfg.LLM.ProviderDisplayName(provider), err)
	}

	// Check 8: Transcripts directory exists
	total++
	transcriptsDir := config.TranscriptsDir()
	if info, err := os.Stat(transcriptsDir); err == nil && info.IsDir() {
		fmt.Println("[OK] Transcripts directory exists")
		passed++
	} else {
		fmt.Printf("[FAIL] Transcripts directory doesn't exist: %s\n", transcriptsDir)
	}

	// Print summary
	fmt.Printf("\n%d/%d checks passed\n", passed, total)

	// Return error if critical checks failed
	if passed < total {
		return fmt.Errorf("health check failed: %d/%d checks passed", passed, total)
	}

	return nil
}
