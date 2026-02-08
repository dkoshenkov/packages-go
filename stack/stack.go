package stack

type Stack[T any] struct {
	data []T
}

func New[T any]() *Stack[T] {
	return &Stack[T]{}
}

func NewCap[T any](cap int) (*Stack[T], error) {
	if cap < 0 {
		return nil, ErrNegativeCapacity
	}
	return &Stack[T]{
		data: make([]T, 0, cap),
	}, nil
}

func (s *Stack[T]) Len() int {
	return len(s.data)
}

func (s *Stack[T]) Push(val T) {
	s.data = append(s.data, val)
}

func (s *Stack[T]) Pop() (T, bool) {
	res, ok := s.Peek()
	if !ok {
		return *new(T), ok
	}
	s.data[s.Len()-1] = *new(T)
	s.data = s.data[:s.Len()-1]
	return res, true
}

func (s *Stack[T]) Peek() (T, bool) {
	if s.Len() == 0 {
		return *new(T), false
	}
	res := s.data[s.Len()-1]
	return res, true
}
