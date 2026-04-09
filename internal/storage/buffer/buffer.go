package buffer

import (
	"errors"
	"fmt"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type PageStore interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, page *disk.Page) error
}

type DirtyPageWriter interface {
	StagePage(id disk.PageID, page *disk.Page) error
	FlushPage(id disk.PageID) error
}

type buffer struct {
	capacity int
	pages    map[disk.PageID]*disk.Page
	store    PageStore
	policy   EvictionPolicy
	writer   DirtyPageWriter
}

func (b *buffer) ReadPage(id disk.PageID) (*disk.Page, error) {
	if page, ok := b.pages[id]; ok {
		if b.policy != nil {
			b.policy.Touch(id)
		}
		return page, nil
	}

	newPage, err := b.store.ReadPage(id)
	if err != nil {
		return nil, err
	}

	if err := b.ensureCapacity(); err != nil {
		return nil, err
	}

	b.pages[id] = newPage
	if b.policy != nil {
		b.policy.Add(id)
	}

	return newPage, nil
}

func (b *buffer) WritePage(id disk.PageID, newData []byte) error {
	page, err := b.ReadPage(id)
	if err != nil {
		return err
	}

	return page.Reset(newData)
}

func (b *buffer) Flush(id disk.PageID) error {
	page, ok := b.pages[id]
	if !ok {
		return errors.New("page not in buffer")
	}
	if !page.IsDirty() {
		return nil
	}

	if b.writer != nil {
		if err := b.writer.StagePage(id, page); err != nil {
			return err
		}
		return b.writer.FlushPage(id)
	}

	return b.store.WritePage(id, page)
}

func (b *buffer) FlushAll() error {
	for id := range b.pages {
		if err := b.Flush(id); err != nil {
			return err
		}
	}

	return nil
}

func (b *buffer) Pin(id disk.PageID) error {
	if _, ok := b.pages[id]; !ok {
		return errors.New("page not in buffer")
	}
	if b.policy != nil {
		b.policy.Pin(id)
	}

	return nil
}

func (b *buffer) Unpin(id disk.PageID) error {
	if _, ok := b.pages[id]; !ok {
		return errors.New("page not in buffer")
	}
	if b.policy != nil {
		b.policy.Unpin(id)
	}

	return nil
}

func (b *buffer) Contains(id disk.PageID) bool {
	_, ok := b.pages[id]
	return ok
}

func (b *buffer) ensureCapacity() error {
	if b.capacity <= 0 {
		return errors.New("buffer capacity must be positive")
	}
	if len(b.pages) < b.capacity {
		return nil
	}
	if b.policy == nil {
		return errors.New("eviction policy is required when buffer is full")
	}

	victimID, err := b.policy.Victim()
	if err != nil {
		return fmt.Errorf("eviction failed: %w", err)
	}
	if err := b.Flush(victimID); err != nil {
		return fmt.Errorf("flush victim %q:%d: %w", victimID.FileName(), victimID.PageNum(), err)
	}

	delete(b.pages, victimID)
	b.policy.Remove(victimID)
	return nil
}
