package copyonwrite

import (
	"hash/crc32"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

func checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

func clonePage(id disk.PageID, page *disk.Page) (*disk.Page, error) {
	cloned := disk.NewPage(id)
	if err := cloned.Reset(page.Data()); err != nil {
		return nil, err
	}

	return cloned, nil
}
