package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/sessionstate"
)

var tuiCmd *cobra.Command
var tuiNewSession bool

func init() {
	if os.Getenv("TERMIA_WRAPPED") != "1" {
		return
	}

	tuiCmd = &cobra.Command{
		Use:   "tui",
		Short: "Interactive history browser",
		RunE:  tuiRun,
	}
	tuiCmd.Flags().BoolVarP(
		&tuiNewSession,
		"new-session",
		"n",
		false,
		"start TUI in a new conversation session",
	)

	rootCmd.AddCommand(tuiCmd)
}

func tuiRun(cmd *cobra.Command, args []string) error {
	var env map[string]string
	if tuiNewSession {
		env = map[string]string{sessionstate.NewSessionEnvKey: "1"}
	}
	return sendWrapperRPC("tui", env)
}

func sendWrapperRPC(command string, env map[string]string) error {
	sockPath := os.Getenv("TERMIA_SOCK")
	if sockPath == "" {
		return fmt.Errorf("no wrapper socket found; start a Termia-wrapped shell first")
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to connect to wrapper socket: %w", err)
	}
	defer conn.Close()

	cdFile := strings.TrimSpace(os.Getenv("TERMIA_CD_FILE"))
	payload := wrapperRPCPayload{Cmd: command}
	if cdFile != "" {
		payload.CdFile = cdFile
	}
	if cwd, err := os.Getwd(); err == nil {
		payload.Cwd = cwd
	}
	if len(env) > 0 {
		payload.Env = env
	}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("failed to send wrapper command: %w", err)
	}

	return nil
}

type wrapperRPCPayload struct {
	Cmd    string            `json:"cmd"`
	CdFile string            `json:"cd_file,omitempty"`
	Cwd    string            `json:"cwd,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}
