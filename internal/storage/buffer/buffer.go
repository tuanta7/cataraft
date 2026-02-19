package buffer

import (
	"errors"
	"fmt"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type PageEvictor interface {
	OnAccess(id disk.PageID)
	OnEvict() (disk.PageID, error)
	Pin(id disk.PageID)
}

type Buffer struct {
	capacity int
	pages    map[disk.PageID]*disk.Page
	disk     *disk.Adapter
	strategy PageEvictor
}

func NewBuffer(capacity int, adapter *disk.Adapter, strategy PageEvictor) *Buffer {
	return &Buffer{
		capacity: capacity,
		pages:    make(map[disk.PageID]*disk.Page),
		disk:     adapter,
		strategy: strategy,
	}
}

func (b *Buffer) ReadPage(id disk.PageID) (*disk.Page, error) {
	if page, ok := b.pages[id]; ok {
		b.strategy.OnAccess(id)
		return page, nil
	}

	newPage, err := b.disk.ReadPage(id)
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

	b.pages[id] = newPage
	b.strategy.OnAccess(id)

	return newPage, nil
}

func (b *Buffer) WritePage(id disk.PageID, newData []byte) error {
	page, err := b.ReadPage(id)
	if err != nil {
		return err
	}

	return page.Write(newData)
}

func (b *Buffer) Flush(id disk.PageID) error {
	if page, ok := b.pages[id]; ok {
		if !page.IsDirty() {
			return nil
		}

		return b.disk.WritePage(id, page)
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
