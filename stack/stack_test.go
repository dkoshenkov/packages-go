package stack

import "testing"

var _ Interface[int] = (*Stack[int])(nil)

func TestStackContract(t *testing.T) {
	t.Parallel()

	runCommonContract(t, func() Interface[int] {
		return New[int]()
	})
}

func TestStackNewCapContract(t *testing.T) {
	t.Parallel()

	runCapacityConstructorContract(t, func(capacity int) (Interface[int], int, error) {
		s, err := NewCap[int](capacity)
		if s == nil {
			return nil, 0, err
		}
		return s, cap(s.data), err
	})
}
