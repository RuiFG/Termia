package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const timingEnvVar = "TERMIA_STARTUP_TIMING"

var timingMutex sync.Mutex

// Track records elapsed time for a startup step when enabled.
// It returns a closure to call at the end of the step.
func Track(step string, fields map[string]string) func() {
	if os.Getenv(timingEnvVar) != "1" {
		return func() {}
	}

	start := time.Now()
	return func() {
		elapsedMs := time.Since(start).Milliseconds()
		logPath := timingLogPath()

		timingMutex.Lock()
		defer timingMutex.Unlock()

		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer file.Close()

		timestamp := time.Now().Format(time.RFC3339Nano)
		fmt.Fprintf(file, "ts=%s pid=%d step=%s elapsed_ms=%d", timestamp, os.Getpid(), step, elapsedMs)
		fmt.Fprintf(file, " args=[%s]", strings.Join(os.Args, " "))

		if len(fields) > 0 {
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(file, " %s=%s", key, fields[key])
			}
		}

		fmt.Fprintln(file)
	}
}

func timingLogPath() string {
	return filepath.Join(timingTempDir(), "termia-startup-timing.log")
}

func timingTempDir() string {
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return os.TempDir()
}
