package btree

const (
	PageSize          = 4096 // 4KB
	HeaderSize        = 24   // 24B for the node header
	DoubleWritePages  = 128  // double-write buffer: 128 * 4KB = 512KB
	DoubleWriteOffset = 0    // double-write lives at the beginning of the page

	DataOffset = DoubleWritePages * PageSize // data pages start after the double-write buffer

	NodeLeaf     = uint8(0)
	NodeInternal = uint8(1)
)

type Page [PageSize]byte

// NodeHeader is the header of a B+Tree node.
// It occupies the first 24 bytes of every page/node.
type NodeHeader struct {
	Checksum uint32 // 4B
	NodeType uint16 // 2B: 0 for leaf, 1 for internal
	NumKeys  uint16 // 2B: number of keys in the node
	Next     uint64 // 8B: pointer to the next leaf node (only for leaf nodes)
	Parent   uint64 // 8B: pointer to the parent node
	_        uint8  // padding to make the header 24 bytes
}
