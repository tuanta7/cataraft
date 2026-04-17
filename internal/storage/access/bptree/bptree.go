package bptree

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

const (
	DefaultOrder = 4
	treeFileName = "index/bptree.db"

	metaPageNum = 0

	metaMagic = "BPTMETA1"
	nodeMagic = "BPTNODE1"
)

var (
	ErrKeyNotFound = errors.New("b+ tree key not found")
)

type Buffer interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, newData []byte) error
	Flush(id disk.PageID) error
	FlushAll() error
	Pin(id disk.PageID) error
	Unpin(id disk.PageID) error
	Contains(id disk.PageID) bool
}

type Entry struct {
	Key   []byte
	Value []byte
}

type BPlusTree struct {
	mu     sync.RWMutex
	order  int
	root   *node
	nodes  map[uint64]*node
	length int
	nextID atomic.Uint64
	buffer Buffer
}

func New(order int, buffer Buffer) (*BPlusTree, error) {
	if order < 3 {
		return nil, errors.New("b+ tree order must be at least 3")
	}
	if buffer == nil {
		return nil, errors.New("b+ tree buffer is required")
	}

	tree := &BPlusTree{
		order:  order,
		buffer: buffer,
		nodes:  make(map[uint64]*node),
	}
	if err := tree.loadOrInit(); err != nil {
		return nil, err
	}

	return tree, nil
}

func (t *BPlusTree) Order() int {
	return t.order
}

func (t *BPlusTree) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.length
}

func (t *BPlusTree) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("key is required")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	leaf := t.findLeaf(key)
	if leaf == nil {
		return nil, ErrKeyNotFound
	}

	idx, ok := leaf.findKeyIndex(key)
	if !ok {
		return nil, ErrKeyNotFound
	}

	return bytes.Clone(leaf.values[idx]), nil
}

func (t *BPlusTree) Put(key, value []byte) error {
	if len(key) == 0 {
		return errors.New("key is required")
	}
	if value == nil {
		return errors.New("value is required")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	leaf := t.findLeaf(key)
	idx, exists := leaf.findKeyIndex(key)
	if exists {
		leaf.values[idx] = bytes.Clone(value)
		return t.persist()
	}

	leaf.keys = insertBytesAt(leaf.keys, idx, key)
	leaf.values = insertBytesAt(leaf.values, idx, value)
	t.length++

	if leaf.keyCount() > t.maxKeys() {
		t.splitLeaf(leaf)
	}

	return t.persist()
}

func (t *BPlusTree) Delete(key []byte) error {
	if len(key) == 0 {
		return errors.New("key is required")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	leaf := t.findLeaf(key)
	if leaf == nil {
		return ErrKeyNotFound
	}

	idx, ok := leaf.findKeyIndex(key)
	if !ok {
		return ErrKeyNotFound
	}

	leaf.keys = removeBytesAt(leaf.keys, idx)
	leaf.values = removeBytesAt(leaf.values, idx)
	t.length--

	if leaf == t.root {
		return t.persist()
	}
	if idx == 0 && len(leaf.keys) > 0 {
		t.updateAncestorKeysAfterFirstKeyChange(leaf)
	}
	if leaf.keyCount() >= t.minLeafKeys() {
		return t.persist()
	}

	t.rebalanceLeaf(leaf)
	return t.persist()
}

func (t *BPlusTree) Scan(start, end []byte) []Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var leaf *node
	switch {
	case t.root == nil:
		return nil
	case start == nil:
		leaf = t.leftmostLeaf()
	default:
		leaf = t.findLeaf(start)
	}

	entries := make([]Entry, 0)
	for leaf != nil {
		for i := range leaf.keys {
			if start != nil && compareKeys(leaf.keys[i], start) < 0 {
				continue
			}
			if end != nil && compareKeys(leaf.keys[i], end) >= 0 {
				return entries
			}

			entries = append(entries, Entry{
				Key:   bytes.Clone(leaf.keys[i]),
				Value: bytes.Clone(leaf.values[i]),
			})
		}
		leaf = leaf.next
	}

	return entries
}

func (t *BPlusTree) RootKeys() [][]byte {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.root == nil {
		return nil
	}

	keys := make([][]byte, len(t.root.keys))
	for i, key := range t.root.keys {
		keys[i] = bytes.Clone(key)
	}
	return keys
}

func (t *BPlusTree) Height() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	height := 0
	current := t.root
	for current != nil {
		height++
		if current.leaf || len(current.children) == 0 {
			break
		}
		current = current.children[0]
	}

	return height
}

func (t *BPlusTree) findLeaf(key []byte) *node {
	current := t.root
	for current != nil && !current.leaf {
		idx := upperBound(current.keys, key)
		current = current.children[idx]
	}

	return current
}

func (t *BPlusTree) leftmostLeaf() *node {
	current := t.root
	for current != nil && !current.leaf {
		current = current.children[0]
	}

	return current
}

func (t *BPlusTree) splitLeaf(leaf *node) {
	mid := (leaf.keyCount() + 1) / 2
	right := t.newLeafNode()
	right.keys = cloneByteSlices(leaf.keys[mid:])
	right.values = cloneByteSlices(leaf.values[mid:])
	leaf.keys = leaf.keys[:mid]
	leaf.values = leaf.values[:mid]
	right.next = leaf.next
	leaf.next = right

	t.insertIntoParent(leaf, right.keys[0], right)
}

func (t *BPlusTree) splitInternal(current *node) {
	mid := current.keyCount() / 2
	promoted := bytes.Clone(current.keys[mid])
	right := t.newInternalNode()
	right.keys = cloneByteSlices(current.keys[mid+1:])
	right.children = append([]*node(nil), current.children[mid+1:]...)
	for _, child := range right.children {
		child.parent = right
	}

	current.keys = current.keys[:mid]
	current.children = current.children[:mid+1]

	t.insertIntoParent(current, promoted, right)
}

func (t *BPlusTree) insertIntoParent(left *node, separator []byte, right *node) {
	if left.parent == nil {
		root := t.newInternalNode()
		root.keys = [][]byte{bytes.Clone(separator)}
		root.children = []*node{left, right}
		left.parent = root
		right.parent = root
		t.root = root
		return
	}

	parent := left.parent
	childIdx := parent.childIndex(left)
	if childIdx < 0 {
		panic(fmt.Sprintf("parent %d does not reference child %d", parent.id, left.id))
	}

	parent.keys = insertBytesAt(parent.keys, childIdx, separator)
	parent.children = insertNodeAt(parent.children, childIdx+1, right)
	right.parent = parent

	if parent.keyCount() > t.maxKeys() {
		t.splitInternal(parent)
	}
}

func (t *BPlusTree) rebalanceLeaf(leaf *node) {
	parent := leaf.parent
	childIdx := parent.childIndex(leaf)

	if childIdx > 0 {
		left := parent.children[childIdx-1]
		if left.keyCount() > t.minLeafKeys() {
			key := left.keys[left.keyCount()-1]
			value := left.values[left.keyCount()-1]
			left.keys = left.keys[:left.keyCount()-1]
			left.values = left.values[:len(left.values)-1]
			leaf.keys = insertBytesAt(leaf.keys, 0, key)
			leaf.values = insertBytesAt(leaf.values, 0, value)
			parent.keys[childIdx-1] = bytes.Clone(leaf.keys[0])
			return
		}
	}

	if childIdx < len(parent.children)-1 {
		right := parent.children[childIdx+1]
		if right.keyCount() > t.minLeafKeys() {
			leaf.keys = append(leaf.keys, bytes.Clone(right.keys[0]))
			leaf.values = append(leaf.values, bytes.Clone(right.values[0]))
			right.keys = removeBytesAt(right.keys, 0)
			right.values = removeBytesAt(right.values, 0)
			parent.keys[childIdx] = bytes.Clone(right.keys[0])
			return
		}
	}

	if childIdx > 0 {
		left := parent.children[childIdx-1]
		left.keys = append(left.keys, cloneByteSlices(leaf.keys)...)
		left.values = append(left.values, cloneByteSlices(leaf.values)...)
		left.next = leaf.next
		t.removeChildFromParent(parent, childIdx-1, childIdx)
		return
	}

	right := parent.children[childIdx+1]
	leaf.keys = append(leaf.keys, cloneByteSlices(right.keys)...)
	leaf.values = append(leaf.values, cloneByteSlices(right.values)...)
	leaf.next = right.next
	t.removeChildFromParent(parent, childIdx, childIdx+1)
}

func (t *BPlusTree) rebalanceInternal(current *node) {
	if current == t.root {
		if len(current.children) == 1 {
			t.root = current.children[0]
			t.root.parent = nil
		}
		return
	}

	parent := current.parent
	childIdx := parent.childIndex(current)

	if childIdx > 0 {
		left := parent.children[childIdx-1]
		if len(left.children) > t.minChildren() {
			separator := bytes.Clone(parent.keys[childIdx-1])
			borrowedChild := left.children[len(left.children)-1]
			borrowedKey := bytes.Clone(left.keys[len(left.keys)-1])

			left.children = left.children[:len(left.children)-1]
			left.keys = left.keys[:len(left.keys)-1]

			current.children = insertNodeAt(current.children, 0, borrowedChild)
			current.keys = insertBytesAt(current.keys, 0, separator)
			borrowedChild.parent = current
			parent.keys[childIdx-1] = borrowedKey
			return
		}
	}

	if childIdx < len(parent.children)-1 {
		right := parent.children[childIdx+1]
		if len(right.children) > t.minChildren() {
			separator := bytes.Clone(parent.keys[childIdx])
			borrowedChild := right.children[0]
			borrowedKey := bytes.Clone(right.keys[0])

			right.children = removeNodeAt(right.children, 0)
			right.keys = removeBytesAt(right.keys, 0)

			current.children = append(current.children, borrowedChild)
			current.keys = append(current.keys, separator)
			borrowedChild.parent = current
			parent.keys[childIdx] = borrowedKey
			return
		}
	}

	if childIdx > 0 {
		left := parent.children[childIdx-1]
		left.keys = append(left.keys, bytes.Clone(parent.keys[childIdx-1]))
		left.keys = append(left.keys, cloneByteSlices(current.keys)...)
		left.children = append(left.children, current.children...)
		for _, child := range current.children {
			child.parent = left
		}
		t.removeChildFromParent(parent, childIdx-1, childIdx)
		return
	}

	right := parent.children[childIdx+1]
	current.keys = append(current.keys, bytes.Clone(parent.keys[childIdx]))
	current.keys = append(current.keys, cloneByteSlices(right.keys)...)
	current.children = append(current.children, right.children...)
	for _, child := range right.children {
		child.parent = current
	}
	t.removeChildFromParent(parent, childIdx, childIdx+1)
}

func (t *BPlusTree) removeChildFromParent(parent *node, keyIdx, childIdx int) {
	parent.keys = removeBytesAt(parent.keys, keyIdx)
	parent.children = removeNodeAt(parent.children, childIdx)

	if parent == t.root && len(parent.keys) == 0 {
		t.root = parent.children[0]
		t.root.parent = nil
		return
	}
	if parent == t.root || parent.keyCount() >= t.minInternalKeys() {
		if keyIdx == 0 && len(parent.keys) > 0 {
			t.updateAncestorKeysAfterFirstKeyChange(parent)
		}
		return
	}

	t.rebalanceInternal(parent)
}

func (t *BPlusTree) updateAncestorKeysAfterFirstKeyChange(current *node) {
	parent := current.parent
	child := current
	for parent != nil {
		childIdx := parent.childIndex(child)
		if childIdx > 0 && len(child.keys) > 0 {
			parent.keys[childIdx-1] = bytes.Clone(child.keys[0])
		}
		child = parent
		parent = parent.parent
	}
}

func (t *BPlusTree) newLeafNode() *node {
	n := &node{
		id:     t.nextNodeID(),
		leaf:   true,
		keys:   make([][]byte, 0, t.maxKeys()+1),
		values: make([][]byte, 0, t.maxKeys()+1),
	}
	t.nodes[n.id] = n
	return n
}

func (t *BPlusTree) newInternalNode() *node {
	n := &node{
		id:       t.nextNodeID(),
		keys:     make([][]byte, 0, t.maxKeys()+1),
		children: make([]*node, 0, t.order+1),
	}
	t.nodes[n.id] = n
	return n
}

func (t *BPlusTree) nextNodeID() uint64 {
	return t.nextID.Add(1)
}

func (t *BPlusTree) maxKeys() int {
	return t.order - 1
}

func (t *BPlusTree) minChildren() int {
	return (t.order + 1) / 2
}

func (t *BPlusTree) minInternalKeys() int {
	return t.minChildren() - 1
}

func (t *BPlusTree) minLeafKeys() int {
	return t.order / 2
}

func compareKeys(left, right []byte) int {
	return bytes.Compare(left, right)
}

func lowerBound(keys [][]byte, target []byte) int {
	low, high := 0, len(keys)
	for low < high {
		mid := low + (high-low)/2
		if compareKeys(keys[mid], target) < 0 {
			low = mid + 1
			continue
		}
		high = mid
	}

	return low
}

func upperBound(keys [][]byte, target []byte) int {
	low, high := 0, len(keys)
	for low < high {
		mid := low + (high-low)/2
		if compareKeys(keys[mid], target) <= 0 {
			low = mid + 1
			continue
		}
		high = mid
	}

	return low
}

func insertBytesAt(items [][]byte, idx int, value []byte) [][]byte {
	items = append(items, nil)
	copy(items[idx+1:], items[idx:])
	items[idx] = bytes.Clone(value)
	return items
}

func removeBytesAt(items [][]byte, idx int) [][]byte {
	copy(items[idx:], items[idx+1:])
	items[len(items)-1] = nil
	return items[:len(items)-1]
}

func insertNodeAt(items []*node, idx int, value *node) []*node {
	items = append(items, nil)
	copy(items[idx+1:], items[idx:])
	items[idx] = value
	return items
}

func removeNodeAt(items []*node, idx int) []*node {
	copy(items[idx:], items[idx+1:])
	items[len(items)-1] = nil
	return items[:len(items)-1]
}

func cloneByteSlices(items [][]byte) [][]byte {
	cloned := make([][]byte, len(items))
	for i, item := range items {
		cloned[i] = bytes.Clone(item)
	}
	return cloned
}

func (t *BPlusTree) loadOrInit() error {
	metaPage, err := t.buffer.ReadPage(disk.NewPageID(treeFileName, metaPageNum))
	if err != nil {
		return err
	}

	meta, ok, err := decodeMetaPage(metaPage.Data())
	if err != nil {
		return err
	}
	if !ok {
		t.root = t.newLeafNode()
		return t.persist()
	}
	if int(meta.Order) != t.order {
		return fmt.Errorf("b+ tree order mismatch: got %d, stored %d", t.order, meta.Order)
	}

	t.length = int(meta.Length)
	t.nextID.Store(meta.NextNodeID)

	loadedNodes := make(map[uint64]*node, int(meta.NextNodeID))
	nodeRefs := make(map[uint64]nodeRefs, int(meta.NextNodeID))
	for id := uint64(1); id <= meta.NextNodeID; id++ {
		page, readErr := t.buffer.ReadPage(t.nodePageID(id))
		if readErr != nil {
			return readErr
		}

		decodedNode, refs, hasNode, decodeErr := decodeNodePage(page.Data())
		if decodeErr != nil {
			return decodeErr
		}
		if !hasNode {
			continue
		}

		loadedNodes[id] = decodedNode
		nodeRefs[id] = refs
	}

	if meta.RootNodeID == 0 {
		t.root = t.newLeafNode()
		return t.persist()
	}

	root, ok := loadedNodes[meta.RootNodeID]
	if !ok {
		return fmt.Errorf("b+ tree root page %d not found", meta.RootNodeID)
	}

	for id, current := range loadedNodes {
		refs := nodeRefs[id]
		if refs.parentID != 0 {
			parent, hasParent := loadedNodes[refs.parentID]
			if !hasParent {
				return fmt.Errorf("b+ tree node %d references missing parent %d", id, refs.parentID)
			}
			current.parent = parent
		}
		if refs.nextID != 0 {
			next, hasNext := loadedNodes[refs.nextID]
			if !hasNext {
				return fmt.Errorf("b+ tree node %d references missing next leaf %d", id, refs.nextID)
			}
			current.next = next
		}
		if !current.leaf {
			current.children = make([]*node, len(refs.childIDs))
			for idx, childID := range refs.childIDs {
				child, hasChild := loadedNodes[childID]
				if !hasChild {
					return fmt.Errorf("b+ tree node %d references missing child %d", id, childID)
				}
				current.children[idx] = child
			}
		}
	}

	t.root = root
	t.nodes = loadedNodes
	return nil
}

func (t *BPlusTree) persist() error {
	if t.root == nil {
		return errors.New("b+ tree root is nil")
	}

	reachable := make(map[uint64]*node)
	t.collectReachable(t.root, reachable)
	t.nodes = reachable

	nodeIDs := make([]uint64, 0, len(reachable))
	for id := range reachable {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})

	metaData := encodeMetaPage(metaPage{
		Order:      uint64(t.order),
		Length:     uint64(t.length),
		NextNodeID: t.nextID.Load(),
		RootNodeID: t.root.id,
	})
	if err := t.buffer.WritePage(disk.NewPageID(treeFileName, metaPageNum), metaData); err != nil {
		return err
	}
	if err := t.buffer.Flush(disk.NewPageID(treeFileName, metaPageNum)); err != nil {
		return err
	}

	for _, nodeID := range nodeIDs {
		nodeData, err := encodeNodePage(reachable[nodeID])
		if err != nil {
			return err
		}

		pageID := t.nodePageID(nodeID)
		if err = t.buffer.WritePage(pageID, nodeData); err != nil {
			return err
		}
		if err = t.buffer.Flush(pageID); err != nil {
			return err
		}
	}

	return nil
}

func (t *BPlusTree) collectReachable(current *node, visited map[uint64]*node) {
	if current == nil {
		return
	}
	if _, ok := visited[current.id]; ok {
		return
	}

	visited[current.id] = current
	for _, child := range current.children {
		t.collectReachable(child, visited)
	}
}

func (t *BPlusTree) nodePageID(nodeID uint64) disk.PageID {
	return disk.NewPageID(treeFileName, int64(nodeID))
}

type metaPage struct {
	Order      uint64
	Length     uint64
	NextNodeID uint64
	RootNodeID uint64
}

func encodeMetaPage(meta metaPage) []byte {
	buf := make([]byte, 0, 64)
	buf = append(buf, []byte(metaMagic)...)
	buf = config.ByteOrder.AppendUint16(buf, 1)
	buf = config.ByteOrder.AppendUint64(buf, meta.Order)
	buf = config.ByteOrder.AppendUint64(buf, meta.Length)
	buf = config.ByteOrder.AppendUint64(buf, meta.NextNodeID)
	buf = config.ByteOrder.AppendUint64(buf, meta.RootNodeID)
	return buf
}

func decodeMetaPage(data []byte) (metaPage, bool, error) {
	if len(data) < len(metaMagic) || string(data[:len(metaMagic)]) != metaMagic {
		return metaPage{}, false, nil
	}

	offset := len(metaMagic)
	if len(data) < offset+2+8+8+8+8 {
		return metaPage{}, false, errors.New("invalid b+ tree metadata page size")
	}
	version := config.ByteOrder.Uint16(data[offset:])
	offset += 2
	if version != 1 {
		return metaPage{}, false, fmt.Errorf("unsupported b+ tree metadata version %d", version)
	}

	return metaPage{
		Order:      config.ByteOrder.Uint64(data[offset:]),
		Length:     config.ByteOrder.Uint64(data[offset+8:]),
		NextNodeID: config.ByteOrder.Uint64(data[offset+16:]),
		RootNodeID: config.ByteOrder.Uint64(data[offset+24:]),
	}, true, nil
}

type nodeRefs struct {
	parentID uint64
	nextID   uint64
	childIDs []uint64
}

func encodeNodePage(n *node) ([]byte, error) {
	if n == nil {
		return nil, errors.New("node is required")
	}
	if n.leaf && len(n.values) != len(n.keys) {
		return nil, fmt.Errorf("leaf node %d has mismatched key/value counts", n.id)
	}
	if !n.leaf && len(n.children) != len(n.keys)+1 {
		return nil, fmt.Errorf("internal node %d has invalid child count", n.id)
	}

	buf := make([]byte, 0, 256)
	buf = append(buf, []byte(nodeMagic)...)
	buf = config.ByteOrder.AppendUint16(buf, 1)
	if n.leaf {
		buf = append(buf, byte(1))
	} else {
		buf = append(buf, byte(0))
	}
	buf = append(buf, byte(0))
	buf = config.ByteOrder.AppendUint64(buf, n.id)

	parentID := uint64(0)
	if n.parent != nil {
		parentID = n.parent.id
	}
	nextID := uint64(0)
	if n.next != nil {
		nextID = n.next.id
	}

	buf = config.ByteOrder.AppendUint64(buf, parentID)
	buf = config.ByteOrder.AppendUint64(buf, nextID)
	buf = config.ByteOrder.AppendUint16(buf, uint16(len(n.keys)))
	if n.leaf {
		buf = config.ByteOrder.AppendUint16(buf, uint16(len(n.values)))
	} else {
		buf = config.ByteOrder.AppendUint16(buf, uint16(len(n.children)))
	}

	for _, key := range n.keys {
		if len(key) > int(^uint16(0)) {
			return nil, fmt.Errorf("node %d key too large", n.id)
		}
		buf = config.ByteOrder.AppendUint16(buf, uint16(len(key)))
		buf = append(buf, key...)
	}

	if n.leaf {
		for _, value := range n.values {
			if len(value) > int(^uint16(0)) {
				return nil, fmt.Errorf("node %d value too large", n.id)
			}
			buf = config.ByteOrder.AppendUint16(buf, uint16(len(value)))
			buf = append(buf, value...)
		}
	} else {
		for _, child := range n.children {
			if child == nil {
				return nil, fmt.Errorf("internal node %d has nil child", n.id)
			}
			buf = config.ByteOrder.AppendUint64(buf, child.id)
		}
	}

	return buf, nil
}

func decodeNodePage(data []byte) (*node, nodeRefs, bool, error) {
	if len(data) < len(nodeMagic) || string(data[:len(nodeMagic)]) != nodeMagic {
		return nil, nodeRefs{}, false, nil
	}

	offset := len(nodeMagic)
	if len(data) < offset+2+1+1+8+8+8+2+2 {
		return nil, nodeRefs{}, false, errors.New("invalid b+ tree node page header")
	}
	version := config.ByteOrder.Uint16(data[offset:])
	offset += 2
	if version != 1 {
		return nil, nodeRefs{}, false, fmt.Errorf("unsupported b+ tree node version %d", version)
	}

	leaf := data[offset] == 1
	offset += 2 // include reserved byte

	nodeID := config.ByteOrder.Uint64(data[offset:])
	offset += 8
	parentID := config.ByteOrder.Uint64(data[offset:])
	offset += 8
	nextID := config.ByteOrder.Uint64(data[offset:])
	offset += 8
	keyCount := int(config.ByteOrder.Uint16(data[offset:]))
	offset += 2
	auxCount := int(config.ByteOrder.Uint16(data[offset:]))
	offset += 2

	decoded := &node{
		id:   nodeID,
		leaf: leaf,
		keys: make([][]byte, keyCount),
	}

	for i := 0; i < keyCount; i++ {
		if len(data) < offset+2 {
			return nil, nodeRefs{}, false, errors.New("truncated b+ tree key length")
		}
		keyLen := int(config.ByteOrder.Uint16(data[offset:]))
		offset += 2
		if len(data) < offset+keyLen {
			return nil, nodeRefs{}, false, errors.New("truncated b+ tree key payload")
		}
		decoded.keys[i] = bytes.Clone(data[offset : offset+keyLen])
		offset += keyLen
	}

	refs := nodeRefs{
		parentID: parentID,
		nextID:   nextID,
	}

	if leaf {
		if auxCount != keyCount {
			return nil, nodeRefs{}, false, errors.New("leaf node has invalid value count")
		}
		decoded.values = make([][]byte, auxCount)
		for i := 0; i < auxCount; i++ {
			if len(data) < offset+2 {
				return nil, nodeRefs{}, false, errors.New("truncated b+ tree value length")
			}
			valueLen := int(config.ByteOrder.Uint16(data[offset:]))
			offset += 2
			if len(data) < offset+valueLen {
				return nil, nodeRefs{}, false, errors.New("truncated b+ tree value payload")
			}
			decoded.values[i] = bytes.Clone(data[offset : offset+valueLen])
			offset += valueLen
		}
		return decoded, refs, true, nil
	}

	if auxCount != keyCount+1 {
		return nil, nodeRefs{}, false, errors.New("internal node has invalid child count")
	}
	refs.childIDs = make([]uint64, auxCount)
	for i := 0; i < auxCount; i++ {
		if len(data) < offset+8 {
			return nil, nodeRefs{}, false, errors.New("truncated b+ tree child pointer")
		}
		refs.childIDs[i] = config.ByteOrder.Uint64(data[offset:])
		offset += 8
	}

	return decoded, refs, true, nil
}
