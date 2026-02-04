package buffer

type PageEvictor interface {
	OnAccess(id PageID)
	OnEvict() (PageID, error)
	Pin(id PageID)
}
