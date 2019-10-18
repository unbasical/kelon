// Package util contains helper functions and data structures.
package util

import (
	log "github.com/sirupsen/logrus"
)

// Simple stack of strings.
// Panics if operations (like Pop()) are performed on an empty stack.
type SStack []string

// Check if the stack is empty.
func (s SStack) Empty() bool {
	return len(s) == 0
}

// Push a string to the top of the stack.
func (s SStack) Push(v string) SStack {
	return append(s, v)
}

// Pop the top element of the stack.
func (s SStack) Pop() (SStack, string) {
	l := len(s)
	if l <= 0 {
		panic("Stack is empty!")
	}
	return s[:l-1], s[l-1]
}

// Stack of string slices.
// Panics if operations (like Pop()) are performed on an empty stack.
//
// This stack also contains debug logs.
type OpStack [][]string

// Push a string slice to the top of the stack.
func (s OpStack) Push(v []string) OpStack {
	log.Debugf("%30sOperands len(%d) PUSH(%+v)", "", len(s), v)
	return append(s, v)
}

// Appends a new string to the top slice of the stack.
func (s OpStack) AppendToTop(v string) {
	l := len(s)
	if l <= 0 {
		panic("Stack is empty!")
	}
	s[l-1] = append(s[l-1], v)
	log.Debugf("%30sOperands len(%d) APPEND |%+v <- TOP", "", len(s), s[l-1])
}

// Pop the top slice of the stack.
func (s OpStack) Pop() (OpStack, []string) {
	l := len(s)
	if l <= 0 {
		panic("Stack is empty!")
	}
	log.Debugf("%30sOperands len(%d) POP()", "", len(s))
	return s[:l-1], s[l-1]
}
