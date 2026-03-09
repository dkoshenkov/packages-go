package stack

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ Interface[int] = (*Sync[int])(nil)

func TestSyncConcurrentPush(t *testing.T) {
	t.Parallel()

	const workers = 8
	const perWorker = 500
	const total = workers * perWorker

	s := NewSync[int]()

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		start := w * perWorker
		go func(start int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				s.Push(start + i)
			}
		}(start)
	}
	wg.Wait()

	require.Equal(t, total, s.Len())

	seen := make(map[int]int, total)
	for i := 0; i < total; i++ {
		v, ok := s.Pop()
		require.True(t, ok)
		seen[v]++
	}

	v, ok := s.Pop()
	require.False(t, ok)
	require.Zero(t, v)

	require.Len(t, seen, total)
	for i := 0; i < total; i++ {
		require.Equal(t, 1, seen[i])
	}
}

func TestSyncConcurrentPop(t *testing.T) {
	t.Parallel()

	const workers = 8
	const total = 5000

	s := NewSync[int]()
	for i := 0; i < total; i++ {
		s.Push(i)
	}

	counts := make(map[int]int, total)
	var countsMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				v, ok := s.Pop()
				if !ok {
					return
				}
				countsMu.Lock()
				counts[v]++
				countsMu.Unlock()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 0, s.Len())
	require.Len(t, counts, total)
	for i := 0; i < total; i++ {
		require.Equal(t, 1, counts[i])
	}
}
