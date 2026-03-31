package agent

import "sync"

var consoleOutputMu sync.Mutex

func LockConsoleOutput() {
	consoleOutputMu.Lock()
}

func UnlockConsoleOutput() {
	consoleOutputMu.Unlock()
}
