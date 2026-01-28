package bptree

type Node struct {
	isRoot   bool
	isLeaf   bool
	keys     []string
	pointers []*Node
}
