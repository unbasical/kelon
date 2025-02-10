// Package util contains helper functions and data structures.
package util

import (
	"github.com/pkg/errors"
	"github.com/unbasical/kelon/pkg/constants/logging"
)

// Stack implementation with generics
type Stack[T any] struct {
	values []T
}

// IsEmpty checks if the stack is empty
func (s *Stack[T]) IsEmpty() bool {
	return len(s.values) == 0
}

// Size returns the size of the stack
func (s *Stack[T]) Size() int {
	return len(s.values)
}

// Clear drops all values from the stack
func (s *Stack[T]) Clear() {
	s.values = s.values[:0]
}

// Push adds a value v to the top of the stack
func (s *Stack[T]) Push(v T) {
	s.values = append(s.values, v)
	logging.LogForComponent("Stack").Debugf("%30sStack len(%d) PUSH(%+v)", "", s.Size(), v)
}

// Pop takes the top most value from the stack
// Throws and error if the stack is empty
func (s *Stack[T]) Pop() (T, error) {
	l := len(s.values)
	var v T
	if l <= 0 {
		return v, errors.New("pop failed due to empty stack")
	}

	v = s.values[l-1]
	s.values = s.values[:l-1]
	logging.LogForComponent("Stack").Debugf("%30sStack len(%d) POP()", "", s.Size())
	return v, nil
}

// Peek returns the top most value from the stack without removing it
// Throws and error if the stack is empty
func (s *Stack[T]) Peek() (T, error) {
	l := len(s.values)
	var v T
	if l <= 0 {
		return v, errors.New("peek failed due to empty stack")
	}

	v = s.values[l-1]
	return v, nil
}

// Values returns all stack values as a slice
func (s *Stack[T]) Values() []T {
	return s.values
}

// AppendToTop Appends a new value to the top slice of the stack.
func AppendToTop[T any](s *Stack[[]T], v T) error {
	top, err := s.Pop()
	if err != nil {
		return errors.Wrap(err, "appendToTop failed: ")
	}

	top = append(top, v)
	s.Push(top)
	logging.LogForComponent("Stack").Debugf("%30sStack len(%d) APPEND |%+v <- TOP", "", s.Size(), s.values[s.Size()-1])
	return nil
}

func AppendToTopChecked[T any](component string, s *Stack[[]T], v T) {
	err := AppendToTop(s, v)
	if err != nil {
		logging.LogForComponent(component).Panicf("Error appending to top: %s", err)
	}
}
