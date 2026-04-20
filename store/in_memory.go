package store

import (
	"sync"
	"time"
)

type MemoryStore struct {
	data map[string]*AppState
	lock sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]*AppState),
	}
}

func (s *MemoryStore) Get(sessionID string) (*AppState, error) {
	s.lock.RLock()
	state, ok := s.data[sessionID]
	s.lock.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	// Update last accessed time
	s.lock.Lock()
	state.LastAccessed = time.Now()
	s.lock.Unlock()

	return state, nil
}

func (s *MemoryStore) Set(sessionID string, state *AppState) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if state.LastAccessed.IsZero() {
		state.LastAccessed = time.Now()
	}
	s.data[sessionID] = state
	return nil
}

func (s *MemoryStore) Delete(sessionID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.data, sessionID)
	return nil
}

func (s *MemoryStore) ForEach(fn func(sessionID string, state *AppState) error) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for sessionID, state := range s.data {
		if err := fn(sessionID, state); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemoryStore) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.data = make(map[string]*AppState) // optional: clear all memory
	return nil
}
