package util

import (
	log "github.com/sirupsen/logrus"
)

var inset = "\t\t\t\t\t"

type SStack []string

func (s SStack) Empty() bool {
	return len(s) == 0
}

func (s SStack) Push(v string) SStack {
	return append(s, v)
}

func (s SStack) Pop() (SStack, string) {
	if l := len(s); l > 0 {
		return s[:l-1], s[l-1]
	} else {
		panic("Stack is empty!")
	}
}

type OpStack [][]string

func (s OpStack) Push(v []string) OpStack {
	log.Debugf("%sOperands len(%d) PUSH(%+v)\n", inset, len(s), v)
	return append(s, v)
}

func (s OpStack) AppendToTop(v string) {
	if l := len(s); l > 0 {
		s[l-1] = append(s[l-1], v)
		log.Debugf("%sOperands len(%d) APPEND |%+v <- TOP\n", inset, len(s), s[l-1])
	} else {
		panic("Stack is empty!")
	}
}

func (s OpStack) Pop() (OpStack, []string) {
	if l := len(s); l > 0 {
		log.Debugf("%sOperands len(%d) POP()\n", inset, len(s))
		return s[:l-1], s[l-1]
	} else {
		panic("Stack is empty!")
	}
}
