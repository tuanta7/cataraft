package bptree

type Page struct {
	size   int
	header []byte
	data   []byte
	free   []byte
}

func (p *Page) Encode() []byte {
	return nil
}
