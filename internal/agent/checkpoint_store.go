package agent

import (
	"context"
	"sync"
)

type memoryCheckPointStore struct {
	mu     sync.RWMutex
	values map[string][]byte
}

func newMemoryCheckPointStore() *memoryCheckPointStore {
	return &memoryCheckPointStore{
		values: make(map[string][]byte),
	}
}

func (s *memoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.values[checkPointID]
	if !ok {
		return nil, false, nil
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	return copied, true, nil
}

func (s *memoryCheckPointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]byte, len(checkPoint))
	copy(copied, checkPoint)
	s.values[checkPointID] = copied
	return nil
}
