package sessionstate

import (
	"os"
	"strings"

	"github.com/termia/termia/internal/config"
)

const sessionEnvKey = "TERMIA_SESSION_ID"

func CurrentID() string {
	if sessionID := strings.TrimSpace(os.Getenv(sessionEnvKey)); sessionID != "" {
		return sessionID
	}
	data, err := os.ReadFile(config.CurrentSessionPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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
