package buffer

import (
	"container/list"
	"errors"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type LRUBuffer struct {
	*buffer
}

func NewLRUBuffer(capacity int, store PageStore) *LRUBuffer {
	return &LRUBuffer{
		buffer: &buffer{
			capacity: capacity,
			pages:    make(map[disk.PageID]*disk.Page),
			store:    store,
			policy: &lruEvictionPolicy{
				order:   list.New(),
				entries: make(map[disk.PageID]*list.Element),
				pinned:  make(map[disk.PageID]struct{}),
			},
		},
	}
}

type lruEvictionPolicy struct {
	order   *list.List
	entries map[disk.PageID]*list.Element
	pinned  map[disk.PageID]struct{} // idiomatic set 0 bytes for the value
}

func (p *lruEvictionPolicy) Add(id disk.PageID) {
	if elem, ok := p.entries[id]; ok {
		p.order.MoveToBack(elem)
		return
	}

	p.entries[id] = p.order.PushBack(id)
}

func (p *lruEvictionPolicy) Touch(id disk.PageID) {
	elem, ok := p.entries[id]
	if !ok {
		return
	}

	p.order.MoveToBack(elem)
}

func (p *lruEvictionPolicy) Remove(id disk.PageID) {
	elem, ok := p.entries[id]
	if ok {
		p.order.Remove(elem)
		delete(p.entries, id)
	}
	delete(p.pinned, id)
}

func (p *lruEvictionPolicy) Victim() (disk.PageID, error) {
	for elem := p.order.Front(); elem != nil; elem = elem.Next() {
		id := elem.Value.(disk.PageID)
		if _, pinned := p.pinned[id]; pinned {
			continue
		}

		return id, nil
	}

	return disk.PageID{}, errors.New("no evictable page available")
}

func (p *lruEvictionPolicy) Pin(id disk.PageID) {
	p.pinned[id] = struct{}{}
}

func (p *lruEvictionPolicy) Unpin(id disk.PageID) {
	delete(p.pinned, id)
}
