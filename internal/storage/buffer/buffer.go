package buffer

import (
	"errors"
	"fmt"

	"github.com/tuanta7/cataraft/internal/config"
)

type Buffer struct {
	capacity int
	pages    map[PageID]*Page
	disk     *DiskAdapter
	strategy PageEvictor
}

func NewBuffer(capacity int, strategy PageEvictor, adapter *DiskAdapter) *Buffer {
	return &Buffer{
		capacity: capacity,
		pages:    make(map[PageID]*Page),
		disk:     adapter,
		strategy: strategy,
	}
}

func (b *Buffer) ReadPage(id PageID) (*Page, error) {
	if page, ok := b.pages[id]; ok {
		b.strategy.OnAccess(id)
		return page, nil
	}

	page := &Page{id: id, data: make([]byte, config.PageSize)}
	err := b.disk.ReadPage(id, page.data)
	if err != nil {
		return nil, err
	}

	if len(b.pages) >= b.capacity {
		victimID, err := b.strategy.OnEvict()
		if err != nil {
			return nil, fmt.Errorf("eviction failed: %w", err)
		}
		delete(b.pages, victimID)
	}

	b.pages[id] = page
	b.strategy.OnAccess(id)

	return page, nil
}

func (b *Buffer) WritePage(id PageID, newData []byte) error {
	page, err := b.ReadPage(id)
	if err != nil {
		return err
	}

	return page.Write(newData)
}

func (b *Buffer) Flush(id PageID) error {
	if page, ok := b.pages[id]; ok {
		if !page.isDirty {
			return nil
		}

		return b.disk.WritePage(id, page.data)
	}

	return errors.New("page not in buffer")
}

func (b *Buffer) FlushAll() error {
	for page := range b.pages {
		if err := b.Flush(page); err != nil {
			return err
		}
	}

	return nil
}
