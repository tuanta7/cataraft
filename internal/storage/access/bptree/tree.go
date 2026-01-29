package bptree

type Tree struct {
	root *Node
}

func NewTree() *Tree {
	return &Tree{}
}

func (t *Tree) Insert(key, value []byte) {}

func (t *Tree) Encode() []byte {
	return nil
}
