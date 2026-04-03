package sessionstate

import (
	"os"
	"testing"

	"github.com/adrg/xdg"
	"github.com/termia/termia/internal/config"
)

func TestCurrentIDPrefersPersistedSessionFileOverStaleEnv(t *testing.T) {
	prevDataHome := xdg.DataHome
	xdg.DataHome = t.TempDir()
	t.Cleanup(func() {
		xdg.DataHome = prevDataHome
	})

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs returned error: %v", err)
	}
	if err := os.WriteFile(config.CurrentSessionPath(), []byte("session-file"), 0644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	t.Setenv(sessionEnvKey, "session-env")

	if got := CurrentID(); got != "session-file" {
		t.Fatalf("expected current session file to win over stale env, got %q", got)
	}
}
