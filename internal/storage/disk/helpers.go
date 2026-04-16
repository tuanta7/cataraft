package disk

import (
	"hash/crc32"
)

func Checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

func ClonePage(id PageID, page *Page) (*Page, error) {
	cloned := NewPage(id)
	if err := cloned.Reset(page.Data()); err != nil {
		return nil, err
	}

	return cloned, nil
}
