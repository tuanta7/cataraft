package bptree

import "encoding/binary"

const (
	pageSize  = 4096 // 4KB
	offPageID = 0
)

type Page struct {
	data [pageSize]byte
}

func (p *Page) SetPageID(id uint64) {
	binary.LittleEndian.PutUint64(p.data[offPageID:], id)
}
