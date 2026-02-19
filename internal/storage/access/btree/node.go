package btree

import "encoding/binary"

const (
	HeaderSize = 4
	PageSize   = 8192
	MaxKeySize = 2048
	MaxValSize = 6144

	LeafNode  = 0
	InnerNode = 1
)

type RawNode []byte

func (r RawNode) Type() uint16 {
	return binary.LittleEndian.Uint16(r[0:2])
}

func (r RawNode) KeyCount() uint16 {
	return binary.LittleEndian.Uint16(r[2:4])
}

func (r RawNode) SetHeader(nodeType, keyCount uint16) {
	binary.LittleEndian.PutUint16(r[0:2], nodeType)
	binary.LittleEndian.PutUint16(r[2:4], keyCount)
}

type Node struct {
	nodeType uint16     // 2B
	keyCount uint16     // 2B
	children []uint64   // n * 8B
	offsets  []uint16   // n * 2B
	data     []KeyValue // arbitrary
}

type KeyValue struct {
	keyLength   uint16 // 2B
	valueLength uint16 // 2B
	key         []byte
	value       []byte
}
