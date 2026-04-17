package copyonwrite

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

const (
	// ShadowFileName is the file name for the CoW shadow pages that store the new copy of the modified page or file.
	ShadowFileName = "copyonwrite.pages"

	// ManifestFileName is the file name for the CoW manifest that store references to actual data files.
	ManifestFileName = "copyonwrite.manifest"

	RecordTypePageVersion uint8 = 1

	LengthFieldSize   = 4
	ChecksumFieldSize = 4

	RecordTypeSize     = 1
	SequenceSize       = 8
	ShadowPageNumSize  = 8
	ChecksumSize       = 4
	NameLenSize        = 2
	LogicalPageNumSize = 8

	HeaderSize       = LengthFieldSize + ChecksumFieldSize
	RecordPrefixSize = ChecksumFieldSize + RecordTypeSize + SequenceSize + ShadowPageNumSize + ChecksumSize + NameLenSize + LogicalPageNumSize
)

type Store interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, page *disk.Page) error
	SyncFile(fn string) error
	AppendFile(fn string, data []byte) (int64, error)
	ReadFileAt(fn string, offset int64, buf []byte) (int, error)
	FileSize(fn string) (int64, error)
}

type Adapter struct {
	mu           sync.Mutex
	store        Store
	basePages    map[disk.PageID]*disk.Page
	shadowPages  map[disk.PageID]entry
	nextSequence uint64
	nextShadow   int64
}

func NewAdapter(store Store) (*Adapter, error) {
	if store == nil {
		return nil, errors.New("copy-on-write disk adapter is required")
	}

	buf := &Adapter{
		store:        store,
		basePages:    make(map[disk.PageID]*disk.Page),
		shadowPages:  make(map[disk.PageID]entry),
		nextSequence: 1,
	}
	if err := buf.RecoverAll(); err != nil {
		return nil, err
	}

	return buf, nil
}

func (a *Adapter) ReadPage(id disk.PageID) (*disk.Page, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	a.mu.Lock()
	if page, ok := a.basePages[id]; ok {
		staged, err := disk.ClonePage(id, page)
		a.mu.Unlock()
		if err != nil {
			return nil, err
		}

		staged.MarkClean()
		return staged, nil
	}

	shadowPageEntry, ok := a.shadowPages[id]
	a.mu.Unlock()
	if !ok {
		// read original page from disk
		return a.store.ReadPage(id)
	}

	shadowID := disk.NewPageID(ShadowFileName, shadowPageEntry.ShadowPageNum)
	shadowPage, err := a.store.ReadPage(shadowID)
	if err != nil {
		return nil, err
	}

	if disk.Checksum(shadowPage.Data()) != shadowPageEntry.Checksum {
		return nil, fmt.Errorf("copy-on-write checksum mismatch for %q:%d", id.FileName(), id.PageNum())
	}

	current, err := disk.ClonePage(id, shadowPage)
	if err != nil {
		return nil, err
	}

	current.MarkClean()
	return current, nil
}

func (a *Adapter) WritePage(id disk.PageID, page *disk.Page) error {
	if err := a.stagePage(id, page); err != nil {
		return err
	}

	return a.flushPage(id)
}

func (a *Adapter) stagePage(id disk.PageID, page *disk.Page) error {
	if page == nil {
		return errors.New("page is required")
	}

	if err := id.Validate(); err != nil {
		return err
	}

	staged, err := disk.ClonePage(id, page)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.basePages[id] = staged
	a.mu.Unlock()

	return nil
}

func (a *Adapter) flushPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	page, ok := a.basePages[id]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("page %q:%d is not staged", id.FileName(), id.PageNum())
	}

	sequence := a.nextSequence
	a.nextSequence++
	shadowPageNum := a.nextShadow
	a.nextShadow++
	a.mu.Unlock()

	shadowID := disk.NewPageID(ShadowFileName, shadowPageNum)
	shadowPage, err := disk.ClonePage(shadowID, page)
	if err != nil {
		return err
	}

	if err = a.store.WritePage(shadowID, shadowPage); err != nil {
		return err
	}

	if err = a.store.SyncFile(ShadowFileName); err != nil {
		return err
	}

	r := record{
		PageID: id,
		Entry: entry{
			Sequence:      sequence,
			ShadowPageNum: shadowPageNum,
			Checksum:      disk.Checksum(page.Data()),
		},
	}
	encoded, err := r.Encode()
	if err != nil {
		return err
	}

	if _, err = a.store.AppendFile(ManifestFileName, encoded); err != nil {
		return err
	}

	if err = a.store.SyncFile(ManifestFileName); err != nil {
		return err
	}

	a.mu.Lock()
	a.shadowPages[id] = r.Entry
	delete(a.basePages, id)
	a.mu.Unlock()

	return nil
}

func (a *Adapter) FlushAll() error {
	a.mu.Lock()
	ids := make([]disk.PageID, 0, len(a.basePages))
	for id := range a.basePages {
		ids = append(ids, id)
	}
	a.mu.Unlock()

	for _, id := range ids {
		if err := a.flushPage(id); err != nil {
			return err
		}
	}

	return nil
}

func (a *Adapter) RecoverAll() error {
	index, nextShadow, nextSequence, err := a.loadManifestIndex()
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.shadowPages = index
	a.basePages = make(map[disk.PageID]*disk.Page)
	a.nextShadow = nextShadow
	a.nextSequence = nextSequence
	a.mu.Unlock()

	return nil
}

func (a *Adapter) ResolvePage(id disk.PageID) (disk.PageID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	shadowPageEntry, ok := a.shadowPages[id]
	if !ok {
		return disk.PageID{}, false
	}

	return disk.NewPageID(ShadowFileName, shadowPageEntry.ShadowPageNum), true
}

func (a *Adapter) loadManifestIndex() (map[disk.PageID]entry, int64, uint64, error) {
	size, err := a.store.FileSize(ManifestFileName)
	if err != nil {
		return nil, 0, 0, err
	}

	index := make(map[disk.PageID]entry)
	nextShadow := int64(0)
	nextSequence := uint64(1)
	for offset := int64(0); offset < size; {
		recordLen, ok, err := a.readManifestRecordLength(offset, size)
		if err != nil {
			return nil, 0, 0, err
		}
		if !ok {
			break
		}

		body := make([]byte, recordLen)
		n, err := a.store.ReadFileAt(ManifestFileName, offset+LengthFieldSize, body)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, 0, 0, err
		}
		if n != len(body) {
			break
		}

		var r record
		if err := r.Decode(body); err != nil {
			return nil, 0, 0, err
		}
		current, ok := index[r.PageID]
		if !ok || r.Entry.Sequence > current.Sequence {
			index[r.PageID] = r.Entry
		}
		if r.Entry.ShadowPageNum >= nextShadow {
			nextShadow = r.Entry.ShadowPageNum + 1
		}
		if r.Entry.Sequence >= nextSequence {
			nextSequence = r.Entry.Sequence + 1
		}

		offset += LengthFieldSize + int64(recordLen)
	}

	return index, nextShadow, nextSequence, nil
}

func (a *Adapter) readManifestRecordLength(offset, size int64) (uint32, bool, error) {
	lengthBuf := make([]byte, LengthFieldSize)
	n, err := a.store.ReadFileAt(ManifestFileName, offset, lengthBuf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, false, err
	}

	if n == 0 || n != len(lengthBuf) {
		return 0, false, nil
	}

	recordLen := config.ByteOrder.Uint32(lengthBuf)
	if recordLen == 0 {
		return 0, false, errors.New("invalid zero-length copy-on-write manifest record")
	}

	if offset+LengthFieldSize+int64(recordLen) > size {
		return 0, false, nil
	}

	return recordLen, true, nil
}
