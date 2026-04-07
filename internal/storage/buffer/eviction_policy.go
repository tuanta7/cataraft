package buffer

import "github.com/tuanta7/cataraft/internal/storage/disk"

type EvictionPolicy interface {
	Add(id disk.PageID)
	Touch(id disk.PageID)
	Remove(id disk.PageID)
	Victim() (disk.PageID, error)
	Pin(id disk.PageID)
	Unpin(id disk.PageID)
}
