package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/tui"
	"go.uber.org/zap"
)

var tuiCmd *cobra.Command

func init() {
	if os.Getenv("TERMIA_WRAPPED") != "1" {
		return
	}

	tuiCmd = &cobra.Command{
		Use:   "tui",
		Short: "Interactive history browser",
		RunE:  tuiRun,
	}

	rootCmd.AddCommand(tuiCmd)
}

func tuiRun(cmd *cobra.Command, args []string) error {

	// Open database
	database, err := db.Open(config.DBPath(), logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Send pause RPC to wrapper via Unix socket
	if err := sendWrapperRPC("pause"); err != nil {
		logger.Debug("failed to send pause RPC", zap.Error(err))
	}

	// Call tui.Run — this blocks until TUI exits
	err = tui.Run(database, cfg, logger)

	// After TUI exits, send resume RPC
	if resumeErr := sendWrapperRPC("resume"); resumeErr != nil {
		logger.Debug("failed to send resume RPC", zap.Error(resumeErr))
	}

	return err
}

func sendWrapperRPC(command string) error {
	sockPath := os.Getenv("TERMIA_SOCK")
	if sockPath == "" {
		logger.Debug("no wrapper socket, running standalone")
		return nil
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to dial wrapper socket: %w", err)
	}
	defer conn.Close()

	payload := map[string]string{"cmd": command}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal RPC payload: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("failed to write RPC command: %w", err)
	}

	return nil
}
