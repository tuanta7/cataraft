package buffer

import "errors"

const (
	PageSize = 8192 // 8KB
)

type Encoder interface {
	Encode() []byte
}

type PageID struct {
	fileName string
	pageNum  int64
}

func (i *PageID) offset() int64 {
	return i.pageNum * PageSize
}

type Page struct {
	id      PageID
	data    []byte
	isDirty bool
	lsn     uint64
}

func NewPage(p Encoder) (*Page, error) {
	data := p.Encode()
	if len(data) != PageSize {
		return nil, errors.New("page size not match")
	}

	return &Page{data: data}, nil
}
