package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStack_PushToEmpty(t *testing.T) {
	s := Stack[int]{values: []int{}}
	s.Push(1)
	assert.Equal(t, []int{1}, s.values)
}

func TestStack_Push(t *testing.T) {
	s := Stack[int]{values: []int{1, 2}}
	s.Push(3)
	assert.Equal(t, []int{1, 2, 3}, s.values)
}

func TestStack_Pop(t *testing.T) {
	s := Stack[int]{values: []int{1, 2}}
	v, err := s.Pop()
	assert.NoError(t, err)
	assert.Equal(t, 2, v)
	assert.Equal(t, []int{1}, s.values)
}

func TestStack_PopEmpty(t *testing.T) {
	s := Stack[int]{values: []int{}}
	_, err := s.Pop()
	assert.EqualError(t, err, "pop failed due to empty stack")
}

func TestStack_Peek(t *testing.T) {
	s := Stack[int]{values: []int{1, 2}}
	v, err := s.Peek()
	assert.NoError(t, err)
	assert.Equal(t, 2, v)
	assert.Equal(t, []int{1, 2}, s.values)
}

func TestStack_PeekEmpty(t *testing.T) {
	s := Stack[int]{values: []int{}}
	_, err := s.Peek()
	assert.EqualError(t, err, "peek failed due to empty stack")
}

func TestStack_IsEmptyTrue(t *testing.T) {
	s := Stack[int]{values: []int{}}
	assert.True(t, s.IsEmpty())
}

func TestStack_IsEmptyFalse(t *testing.T) {
	s := Stack[int]{values: []int{1, 2}}
	assert.False(t, s.IsEmpty())
}

func TestStack_Clear(t *testing.T) {
	s := Stack[int]{values: []int{1, 2, 3}}

	s.Clear()
	assert.Empty(t, s.values)
}

func TestAppendToTop_Empty(t *testing.T) {
	s := Stack[[]int]{values: [][]int{{}}}
	err := AppendToTop(&s, 1)
	assert.NoError(t, err)
	assert.Equal(t, [][]int{{1}}, s.values)
}

func TestAppendToTop(t *testing.T) {
	s := Stack[[]int]{values: [][]int{{1, 2}, {3, 4}}}
	err := AppendToTop(&s, 5)
	assert.NoError(t, err)
	assert.Equal(t, [][]int{{1, 2}, {3, 4, 5}}, s.values)
}
