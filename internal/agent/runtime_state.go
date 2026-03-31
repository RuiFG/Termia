package agent

import "sync"

type runtimeState struct {
	mu     sync.RWMutex
	values map[string]any
}

func newRuntimeState() *runtimeState {
	return &runtimeState{
		values: make(map[string]any),
	}
}

func (s *runtimeState) get(key string) (any, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	value, ok := s.values[key]
	s.mu.RUnlock()
	return value, ok
}

func (s *runtimeState) set(key string, value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
}

func (s *runtimeState) snapshot() map[string]any {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.values) == 0 {
		return nil
	}
	out := make(map[string]any, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out
}
