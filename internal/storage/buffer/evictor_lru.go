package buffer

import (
	"container/list"
	"errors"
)

type LRUNode struct {
	id       PageID
	isPinned bool
}

type LRUList struct {
	nodeList list.List
	nodeMap  map[PageID]*list.Element
}

func NewLRUList() *LRUList {
	return &LRUList{}
}

func (f *LRUList) OnEvict() (PageID, error) {
	victim := f.nodeList.Front()
	if victim == nil {
		return PageID{}, errors.New("empty list")
	}
	value := f.nodeList.Remove(victim)

	id := value.(*LRUNode).id
	delete(f.nodeMap, id)

	return id, nil
}

func (f *LRUList) OnAccess(id PageID) {
	node := &LRUNode{
		id:       id,
		isPinned: false,
	}

	if element, exist := f.nodeMap[id]; exist {
		f.nodeList.Remove(element)
	}

	element := f.nodeList.PushBack(node)
	f.nodeMap[id] = element
}

func (f *LRUList) Pin(id PageID) {
	node := f.nodeMap[id]
	node.Value.(*LRUNode).isPinned = true
}
