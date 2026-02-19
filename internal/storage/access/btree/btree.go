package btree

type Tree struct {
	root *Node
}

func NewTree() *Tree {
	return &Tree{}
}

func (t *Tree) Insert(key, value []byte) {}

func (t *Tree) Get(key []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (t *Tree) Delete(key []byte) {
	//TODO implement me
	panic("implement me")
}

func (t *Tree) Encode() []byte {
	return nil
}
