package agent

// CommandExecutedEvents is a no-op placeholder for the old event system.
// Returns a channel that never receives any events.
func CommandExecutedEvents() <-chan struct{} {
	ch := make(chan struct{})
	// Never close or send on this channel - it's a no-op placeholder
	return ch
}
