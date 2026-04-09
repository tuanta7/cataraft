package recovery

import "github.com/tuanta7/cataraft/internal/storage/disk"

type LogRecord struct {
	LSN      uint64
	PageID   disk.PageID
	Offset   uint32
	Payload  []byte
	Checksum uint32
}
