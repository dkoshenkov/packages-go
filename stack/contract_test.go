package stack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testItem struct{ id int }

func runCommonContract(t *testing.T, newStack func() Interface[int]) {
	t.Helper()

	t.Run("empty stack", func(t *testing.T) {
		t.Parallel()

		s := newStack()
		require.NotNil(t, s)
		require.Equal(t, 0, s.Len())

		v, ok := s.Peek()
		require.False(t, ok)
		require.Zero(t, v)

		v, ok = s.Pop()
		require.False(t, ok)
		require.Zero(t, v)
	})

	t.Run("lifo and len", func(t *testing.T) {
		t.Parallel()

		s := newStack()
		require.NotNil(t, s)

		input := []int{10, 20, 30}
		for i, v := range input {
			s.Push(v)
			require.Equal(t, i+1, s.Len())
		}

		v, ok := s.Peek()
		require.True(t, ok)
		require.Equal(t, 30, v)

		wantPop := []int{30, 20, 10}
		for i, want := range wantPop {
			got, ok := s.Pop()
			require.True(t, ok)
			require.Equal(t, want, got)
			require.Equal(t, len(wantPop)-i-1, s.Len())
		}

		v, ok = s.Pop()
		require.False(t, ok)
		require.Zero(t, v)
	})
}

func runCapacityConstructorContract(
	t *testing.T,
	ctor func(capacity int) (s Interface[int], actualCap int, err error),
) {
	t.Helper()

	t.Run("negative capacity", func(t *testing.T) {
		t.Parallel()

		s, _, err := ctor(-1)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNegativeCapacity)
		require.Nil(t, s)
	})

	t.Run("zero capacity", func(t *testing.T) {
		t.Parallel()

		s, actualCap, err := ctor(0)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.Equal(t, 0, s.Len())
		require.Equal(t, 0, actualCap)
	})

	t.Run("positive capacity", func(t *testing.T) {
		t.Parallel()

		const wantCap = 8
		s, actualCap, err := ctor(wantCap)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.Equal(t, 0, s.Len())
		require.Equal(t, wantCap, actualCap)
	})
}

func runPopClearsRemovedSlot(
	t *testing.T,
	push func(*testItem),
	pop func() (*testItem, bool),
	removedSlot func() *testItem,
) {
	t.Helper()

	first := &testItem{id: 1}
	second := &testItem{id: 2}
	push(first)
	push(second)

	got, ok := pop()
	require.True(t, ok)
	require.Same(t, second, got)
	require.Nil(t, removedSlot())
}

func TestPopClearsRemovedSlot(t *testing.T) {
	t.Parallel()

	type harness struct {
		push        func(*testItem)
		pop         func() (*testItem, bool)
		removedSlot func() *testItem
	}

	tests := []struct {
		name string
		new  func(t *testing.T) harness
	}{
		{
			name: "stack",
			new: func(t *testing.T) harness {
				t.Helper()
				s := New[*testItem]()
				return harness{
					push: func(v *testItem) { s.Push(v) },
					pop:  func() (*testItem, bool) { return s.Pop() },
					removedSlot: func() *testItem {
						require.GreaterOrEqual(t, cap(s.data), 2)
						return s.data[:cap(s.data)][1]
					},
				}
			},
		},
		{
			name: "sync",
			new: func(t *testing.T) harness {
				t.Helper()
				s := NewSync[*testItem]()
				return harness{
					push: func(v *testItem) { s.Push(v) },
					pop:  func() (*testItem, bool) { return s.Pop() },
					removedSlot: func() *testItem {
						require.GreaterOrEqual(t, cap(s.data), 2)
						return s.data[:cap(s.data)][1]
					},
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := tt.new(t)
			runPopClearsRemovedSlot(t, h.push, h.pop, h.removedSlot)
		})
	}
}
