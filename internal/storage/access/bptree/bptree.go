package btree

import (
	"sync"
	"sync/atomic"
)

type BPlusTree struct {
	mu     sync.RWMutex
	rootID atomic.Uint64
}
