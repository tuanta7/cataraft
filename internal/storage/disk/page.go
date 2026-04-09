package disk

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tuanta7/cataraft/internal/config"
)

type Page struct {
	id      PageID
	isDirty bool
	lsn     uint64 // log sequence number
	data    []byte
}

func NewPage(id PageID) *Page {
	return &Page{
		id:   id,
		data: make([]byte, config.PageSize),
	}
}

func (p *Page) Write(data []byte) error {
	return p.WriteAt(0, data)
}

func (p *Page) WriteAt(offset int, data []byte) error {
	if offset < 0 || offset > config.PageSize {
		return fmt.Errorf("invalid write offset %d", offset)
	}

	if offset+len(data) > config.PageSize {
		return errors.New("page size exceeded")
	}

	if p.data == nil || len(p.data) != config.PageSize {
		p.data = make([]byte, config.PageSize)
	}

	copy(p.data[offset:], data)
	p.isDirty = true
	return nil
}

// Reset the page with the given data.
func (p *Page) Reset(data []byte) error {
	if len(data) > config.PageSize {
		return errors.New("page size exceeded")
	}

	if p.data == nil || len(p.data) != config.PageSize {
		p.data = make([]byte, config.PageSize)
	}

	clear(p.data)
	copy(p.data, data)

	p.isDirty = true
	return nil
}

func (p *Page) IsDirty() bool {
	return p.isDirty
}

func (p *Page) MarkClean() {
	p.isDirty = false
}

func (p *Page) ID() PageID {
	return p.id
}

func (p *Page) LSN() uint64 {
	return p.lsn
}

func (p *Page) SetLSN(lsn uint64) {
	p.lsn = lsn
}

func (p *Page) Data() []byte {
	// Return a copy of the data to prevent external modification.
	return bytes.Clone(p.data)
}
