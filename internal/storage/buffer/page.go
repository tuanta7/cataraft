package buffer

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
	data    []byte
	isDirty bool
	lsn     uint64
}

func (p *Page) Write(data []byte) error {
	if len(data) > config.PageSize {
		return errors.New("page size exceeded")
	}

	// fill the rest of the page with zeros
	p.data = append(data, make([]byte, config.PageSize-len(data))...)
	p.isDirty = true
	return nil
}
