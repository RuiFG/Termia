//go:build !windows

package wrapper

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/tui"
	"go.uber.org/zap"
	"golang.org/x/term"
)

type wrapperCommand struct {
	Cmd    string            `json:"cmd"`
	CdFile string            `json:"cd_file,omitempty"`
	Cwd    string            `json:"cwd,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

func (w *Wrapper) socketPath() string {
	if w.sockPath != "" {
		return w.sockPath
	}

	w.sockPath = filepath.Join(config.TermiaDir(), fmt.Sprintf("termia-%d.sock", os.Getpid()))
	return w.sockPath
}

func (w *Wrapper) startServer() error {
	sockPath := w.socketPath()
	if err := os.RemoveAll(sockPath); err != nil {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	if err := os.Chmod(sockPath, 0600); err != nil {
		w.logger.Warn("failed to chmod wrapper socket", zap.Error(err))
	}

	w.sockListener = listener
	go w.acceptLoop()

	return nil
}

func (w *Wrapper) stopServer() {
	if w.sockListener != nil {
		_ = w.sockListener.Close()
		w.sockListener = nil
	}

	if w.sockPath != "" {
		_ = os.Remove(w.sockPath)
	}
}

func (w *Wrapper) acceptLoop() {
	for {
		conn, err := w.sockListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			select {
			case <-w.done:
				return
			default:
			}
			w.logger.Warn("wrapper socket accept error", zap.Error(err))
			return
		}

		go w.handleConn(conn)
	}
}

func (w *Wrapper) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var cmd wrapperCommand
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			w.logger.Warn("failed to parse wrapper command", zap.Error(err))
			continue
		}

		w.handleCommand(cmd)
	}

	if err := scanner.Err(); err != nil {
		w.logger.Warn("wrapper socket read error", zap.Error(err))
	}
}

func (w *Wrapper) handleCommand(cmd wrapperCommand) {
	switch strings.ToLower(strings.TrimSpace(cmd.Cmd)) {
	case "pause":
		w.Pause()
	case "resume":
		w.Resume()
	case "tui":
		w.startTUI(strings.TrimSpace(cmd.CdFile), strings.TrimSpace(cmd.Cwd), cmd.Env)
	default:
		w.logger.Warn("unknown wrapper command", zap.String("cmd", cmd.Cmd))
	}
}

func (w *Wrapper) startTUI(cdFile, cwd string, env map[string]string) {
	w.tuiMu.Lock()
	if w.tuiActive {
		w.tuiMu.Unlock()
		w.logger.Debug("tui already active")
		return
	}
	w.tuiActive = true
	w.tuiMu.Unlock()

	go func(cdFile, cwd string, env map[string]string) {
		defer func() {
			w.tuiMu.Lock()
			w.tuiActive = false
			w.tuiMu.Unlock()
		}()

		if err := w.runTUI(cdFile, cwd, env); err != nil {
			w.logger.Error("tui run failed", zap.Error(err))
		}
	}(cdFile, cwd, env)
}

func (w *Wrapper) runTUI(cdFile, cwd string, env map[string]string) error {
	w.Pause()
	defer w.Resume()
	prevCwd, _ := os.Getwd()
	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			w.logger.Warn("failed to set tui cwd", zap.Error(err))
		}
	}
	defer func() {
		if prevCwd == "" {
			return
		}
		if err := os.Chdir(prevCwd); err != nil {
			w.logger.Warn("failed to restore wrapper cwd", zap.Error(err))
		}
	}()
	prevEnv := map[string]string{}
	for key, value := range env {
		prevEnv[key] = os.Getenv(key)
		if strings.TrimSpace(value) == "" {
			_ = os.Unsetenv(key)
			continue
		}
		_ = os.Setenv(key, value)
	}
	defer func() {
		for key, value := range prevEnv {
			if value == "" {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, value)
		}
	}()
	prevActive := os.Getenv("TERMIA_TUI_ACTIVE")
	_ = os.Setenv("TERMIA_TUI_ACTIVE", "1")
	defer func() {
		if prevActive == "" {
			_ = os.Unsetenv("TERMIA_TUI_ACTIVE")
			return
		}
		_ = os.Setenv("TERMIA_TUI_ACTIVE", prevActive)
	}()
	prevCdFile := os.Getenv("TERMIA_CD_FILE")
	if cdFile == "" {
		_ = os.Unsetenv("TERMIA_CD_FILE")
	} else {
		_ = os.Setenv("TERMIA_CD_FILE", cdFile)
	}
	defer func() {
		if prevCdFile == "" {
			_ = os.Unsetenv("TERMIA_CD_FILE")
			return
		}
		_ = os.Setenv("TERMIA_CD_FILE", prevCdFile)
	}()

	if w.oldState != nil {
		if err := term.Restore(int(os.Stdin.Fd()), w.oldState); err != nil {
			w.logger.Warn("failed to restore terminal before tui", zap.Error(err))
		}
	}

	cfg := w.cfg
	if cfg == nil {
		var err error
		cfg, err = config.LoadOrDefault()
		if err != nil {
			return err
		}
	}

	tuiErr := tui.Run(w.db, cfg, w.logger)
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		if tuiErr != nil {
			w.logger.Error("failed to re-enter raw mode after tui", zap.Error(err))
		} else {
			tuiErr = fmt.Errorf("failed to re-enter raw mode after tui: %w", err)
		}
	}

	return tuiErr
}
