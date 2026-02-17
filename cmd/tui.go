package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
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
	return sendWrapperRPC("tui")
}

func sendWrapperRPC(command string) error {
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
	env := collectLLMEnv(cfg)
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

func collectLLMEnv(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	env := make(map[string]string)
	addEnvValue(env, cfg.LLM.OpenAI.APIKeyEnv)
	addEnvValue(env, cfg.LLM.Anthropic.APIKeyEnv)
	addEnvValue(env, cfg.LLM.Ollama.APIKeyEnv)
	addEnvValue(env, cfg.LLM.DeepSeek.APIKeyEnv)
	if len(env) == 0 {
		return nil
	}
	return env
}

func addEnvValue(values map[string]string, key string) {
	if values == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	values[key] = value
}
