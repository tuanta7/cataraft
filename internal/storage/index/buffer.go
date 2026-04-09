package index

import "github.com/tuanta7/cataraft/internal/storage/disk"

type Buffer interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, newData []byte) error
	Flush(id disk.PageID) error
	FlushAll() error
	Pin(id disk.PageID) error
	Unpin(id disk.PageID) error
	Contains(id disk.PageID) bool
}
