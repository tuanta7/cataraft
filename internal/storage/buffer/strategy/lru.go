package strategy

import "container/list"

type LRUList[T comparable] struct {
	nodes list.List
}

func NewLRUList[T comparable]() *LRUList[T] {
	return &LRUList[T]{}
}

func (f *LRUList[T]) OnEvict() (T, error) {
	var t T
	return t, nil
}

func (f *LRUList[T]) OnAccess(id T) {
	f.nodes.PushBack(id)
}
