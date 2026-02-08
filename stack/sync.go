package stack

import (
	"sync"
)

type Sync[T any] struct {
	data []T
	mu   sync.RWMutex
}

func NewSync[T any]() *Sync[T] {
	return &Sync[T]{
		data: make([]T, 0),
		mu:   sync.RWMutex{},
	}
}

func NewSyncStackCap[T any](cap int) (*Sync[T], error) {
	if cap < 0 {
		return nil, ErrNegativeCapacity
	}
	return &Sync[T]{
		data: make([]T, 0, cap),
		mu:   sync.RWMutex{},
	}, nil
}

func (s *Sync[T]) Push(v T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = append(s.data, v)
}

func (s *Sync[T]) Peek() (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peek()
}

func (s *Sync[T]) Pop() (v T, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.peek()
	if !ok {
		return *new(T), false
	}
	s.data[s.len()-1] = res
	s.data = s.data[:s.len()-1]
	return res, true
}

func (s *Sync[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l := s.len()
	return l
}

func (s *Sync[T]) peek() (T, bool) {
	if s.empty() {
		return *new(T), false
	}
	return s.data[len(s.data)-1], true
}

func (s *Sync[T]) len() int {
	return len(s.data)
}

func (s *Sync[T]) empty() bool {
	return s.len() == 0
}
