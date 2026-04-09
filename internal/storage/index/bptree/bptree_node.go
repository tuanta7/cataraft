package bptree

type node struct {
	id       uint64
	leaf     bool
	keys     [][]byte
	values   [][]byte
	children []*node
	parent   *node
	next     *node
}

func (n *node) keyCount() int {
	return len(n.keys)
}

func (n *node) childIndex(child *node) int {
	for i, candidate := range n.children {
		if candidate == child {
			return i
		}
	}

	return -1
}

func (n *node) findKeyIndex(key []byte) (int, bool) {
	idx := lowerBound(n.keys, key)
	if idx < len(n.keys) && compareKeys(n.keys[idx], key) == 0 {
		return idx, true
	}

	return idx, false
}
