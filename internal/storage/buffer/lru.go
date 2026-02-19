package buffer

import (
	"container/list"
	"errors"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type LRUNode struct {
	id       disk.PageID
	isPinned bool
}

type LRUList struct {
	nodeList list.List
	nodeMap  map[disk.PageID]*list.Element
}

func NewLRUList() *LRUList {
	return &LRUList{}
}

func (f *LRUList) OnEvict() (disk.PageID, error) {
	victim := f.nodeList.Front()
	if victim == nil {
		return disk.PageID{}, errors.New("empty list")
	}
	value := f.nodeList.Remove(victim)

	id := value.(*LRUNode).id
	delete(f.nodeMap, id)

	return id, nil
}

func (f *LRUList) OnAccess(id disk.PageID) {
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

func (f *LRUList) Pin(id disk.PageID) {
	node := f.nodeMap[id]
	node.Value.(*LRUNode).isPinned = true
}
