package disk

import (
	"errors"

	"github.com/tuanta7/cataraft/internal/config"
)

type PageID struct {
	fileName string
	pageNum  int64
}

func (i *PageID) offset() int64 {
	return i.pageNum * config.PageSize
}

type Page struct {
	id      PageID
	isDirty bool
	lsn     uint64
	data    []byte
}

func NewPage(id PageID) *Page {
	return &Page{
		id:   id,
		data: make([]byte, config.PageSize),
	}
}

func (p *Page) Write(data []byte) error {
	if len(data) > config.PageSize {
		return errors.New("page size exceeded")
	}

	if p.data == nil || len(p.data) != config.PageSize {
		p.data = make([]byte, config.PageSize)
	}

	copy(p.data, data)
	if len(data) < config.PageSize {
		// ensure tail is zeroed
		for i := len(data); i < config.PageSize; i++ {
			p.data[i] = 0
		}
	}

	p.isDirty = true
	return nil
}

func (p *Page) IsDirty() bool {
	return p.isDirty
}
