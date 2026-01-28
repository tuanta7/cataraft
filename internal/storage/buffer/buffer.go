package buffer

type Evictor interface {
	Get() *PageID
	Put(id *PageID)
}

type Buffer struct {
	pages   map[PageID]*Page
	disk    *DiskAdapter
	evictor Evictor
}

func NewBuffer(da *DiskAdapter, evictor Evictor) *Buffer {
	return &Buffer{
		disk:    da,
		pages:   make(map[PageID]*Page),
		evictor: evictor,
	}
}

func (b *Buffer) GetPage(id PageID) (*Page, error) {
	if page, ok := b.pages[id]; ok {
		return page, nil
	}

	return nil, nil
}

func (b *Buffer) Write(id PageID, page *Page) error {
	return nil
}

func (b *Buffer) Flush(id PageID) error {
	return nil
}
