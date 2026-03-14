// Package stack provides LIFO stacks.
//
// A stack is a last-in-first-out collection: the most recently pushed element
// is the first one returned by Pop or Peek.
package stack

// Interface stack describes a generic LIFO stack behavior.
//
// Pop and Peek are total functions: they never panic due to an empty stack.
// If the stack is empty, they return the zero value of T and ok=false.
// Otherwise, they return the top element and ok=true.
type Interface[T any] interface {
	// Push adds v to the top of the stack.
	Push(v T)

	// Pop removes and returns the top element of the stack.
	//
	// If the stack is empty, it returns the zero value of T and ok=false.
	Pop() (v T, ok bool)

	// Peek returns the top element of the stack without removing it.
	//
	// If the stack is empty, it returns the zero value of T and ok=false.
	Peek() (v T, ok bool)

	// Len returns the number of elements currently in the stack.
	Len() int
}
