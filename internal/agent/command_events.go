package agent

var commandEventChan = make(chan struct{}, 8)

func notifyCommandExecuted() {
	select {
	case commandEventChan <- struct{}{}:
	default:
	}
}

func CommandExecutedEvents() <-chan struct{} {
	return commandEventChan
}
