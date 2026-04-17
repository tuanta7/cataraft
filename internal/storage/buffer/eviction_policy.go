package buffer

import "github.com/tuanta7/cataraft/internal/storage/disk"

type EvictionPolicy interface {
	// Add inserts a page into policy's internal data structures.
	Add(id disk.PageID)
	// Touch indicates that the page was recently used and its priority should be updated.
	Touch(id disk.PageID)
	// Remove deletes a page from the eviction policy.
	Remove(id disk.PageID)
	// Victim returns an eviction candidate.
	Victim() (disk.PageID, error)
	// Pin marks a page as non-evictable.
	Pin(id disk.PageID)
	// Unpin marks a page as evictable again.
	Unpin(id disk.PageID)
}
