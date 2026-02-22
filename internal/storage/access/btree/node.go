package btree

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/tuanta7/cataraft/internal/config"
)

const (
	HeaderSize  = 4
	PointerSize = 8
	OffsetSize  = 2

	MaxKeySize = 2000
	MaxValSize = 6000
	PageSize   = 8192

	LeafNode  = 0
	InnerNode = 1
)

var (
	ErrInvalidNode     = errors.New("invalid node")
	ErrIndexOutOfRange = errors.New("index out of range")
)

type Node []byte

func NewNode(nodeType, keyCount uint16) Node {
	node := make(Node, config.PageSize)
	node.SetHeader(nodeType, keyCount)
	return node
}

func (n Node) Type() uint16 {
	return binary.LittleEndian.Uint16(n[0:2])
}

func (n Node) KeyCount() uint16 {
	return binary.LittleEndian.Uint16(n[2:4])
}

func (n Node) SetHeader(nodeType, keyCount uint16) {
	binary.LittleEndian.PutUint16(n[0:2], nodeType)
	binary.LittleEndian.PutUint16(n[2:4], keyCount)
}

func (n Node) GetChildPointer(idx uint16) (uint64, error) {
	if idx >= n.KeyCount() {
		return 0, ErrIndexOutOfRange
	}

	pointerOffset := HeaderSize + idx*8
	rawPointer := n[pointerOffset : pointerOffset+8]

	return binary.LittleEndian.Uint64(rawPointer), nil
}

func (n Node) SetChildPointer(idx uint16, pointer uint64) {
	pointerOffset := HeaderSize + idx*8
	rawPointer := n[pointerOffset : pointerOffset+8] // share the same underlying array
	binary.LittleEndian.PutUint64(rawPointer, pointer)
}

func (n Node) getOffset(idx uint16) uint16 {
	if idx == 0 {
		// the first key-value pair is always at the start of the data section
		return 0
	}

	// for the follow key-value pair, read the stored offset for fast access
	// offset[i] stores the end offset of the (i-1) key-value pair
	offsetOffset := HeaderSize + (idx-1)*2
	rawOffset := n[offsetOffset : offsetOffset+2]
	return binary.LittleEndian.Uint16(rawOffset)
}

func (n Node) getKVOffset(idx uint16) (uint16, error) {
	keyCount := n.KeyCount()
	if idx > keyCount {
		return 0, ErrIndexOutOfRange
	}

	return HeaderSize + keyCount*8 + keyCount*2 + n.getOffset(idx), nil
}

func (n Node) GetKey(idx uint16) ([]byte, error) {
	kvOffset, err := n.getKVOffset(idx)
	if err != nil {
		return nil, err
	}

	keyLength := binary.LittleEndian.Uint16(n[kvOffset : kvOffset+2])
	return n[kvOffset+4 : kvOffset+4+keyLength], nil
}

func (n Node) GetValue(idx uint16) ([]byte, error) {
	kvOffset, err := n.getKVOffset(idx)
	if err != nil {
		return nil, err
	}

	keyLength := binary.LittleEndian.Uint16(n[kvOffset : kvOffset+2])
	valueLength := binary.LittleEndian.Uint16(n[kvOffset+2 : kvOffset+4])
	return n[kvOffset+4+keyLength : kvOffset+4+keyLength+valueLength], nil
}

func (n Node) UsedSpace() uint16 {
	// since offset[i] stores the end offset of the i key-value pair,
	// the total used space is the end offset of the last key-value pair
	size, _ := n.getKVOffset(n.KeyCount())
	return size
}

// SeekLE returns the first kid node whose range intersects the key, in other words,
// find the largest key in the node that is less than or equal to the given key, and return its child pointer.
func (n Node) SeekLE(key []byte) uint16 {
	keyCount := n.KeyCount()
	found := uint16(0)

	// TODO: use binary search
	for i := uint16(1); i < keyCount; i++ {
		k, _ := n.GetKey(i)

		if cmp := bytes.Compare(k, key); cmp <= 0 {
			found = i
		} else {
			break
		}
	}

	return found
}

func (n Node) Decode() (*ParsedNode, error) {
	return &ParsedNode{
		nodeType: n.Type(),
		keyCount: n.KeyCount(),
	}, nil
}

func (n Node) leafInsert() {

}
