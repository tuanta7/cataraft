package buffer

import "container/list"

type LRU struct {
	capacity uint64
	list     list.List
}

func NewLRU(capacity uint64) *LRU {
	return &LRU{
		capacity: capacity,
	}
}
