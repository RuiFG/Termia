package sessionstate

import (
	"os"
	"strconv"
	"strings"

	"github.com/termia/termia/internal/config"
)

const (
	sessionEnvKey    = "TERMIA_SESSION_ID"
	NewSessionEnvKey = "TERMIA_NEW_SESSION"
)

func CurrentID() string {
	data, err := os.ReadFile(config.CurrentSessionPath())
	if err == nil {
		if sessionID := strings.TrimSpace(string(data)); sessionID != "" {
			return sessionID
		}
	}
	if sessionID := strings.TrimSpace(os.Getenv(sessionEnvKey)); sessionID != "" {
		return sessionID
	}
	return ""
}

func SetCurrentID(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		_ = os.Unsetenv(sessionEnvKey)
		err := os.Remove(config.CurrentSessionPath())
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	if err := os.WriteFile(config.CurrentSessionPath(), []byte(sessionID), 0644); err != nil {
		return err
	}
	return os.Setenv(sessionEnvKey, sessionID)
}

func NewSessionRequested() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(NewSessionEnvKey)))
	return err == nil && enabled
}
