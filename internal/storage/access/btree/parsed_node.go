package btree

type ParsedNode struct {
	nodeType uint16     // 2B
	keyCount uint16     // 2B
	pointers []uint64   // n * 8B
	offsets  []uint16   // n * 2B
	data     []ParsedKV // arbitrary
	unused   []byte
}

type ParsedKV struct {
	keyLength   uint16 // 2B
	valueLength uint16 // 2B
	key         []byte
	value       []byte
}
