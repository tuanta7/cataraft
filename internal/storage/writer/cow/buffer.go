package cow

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/recovery"
	"github.com/tuanta7/cataraft/internal/storage/writer"
)

const (
	// ShadowFileName is the file name for the copy-on-write shadow pages,
	// which are used to store the current state of the pages before they are written to the main log.
	ShadowFileName = "copyonwrite.pages"

	// ManifestFileName is the file name for the copy-on-write manifest,
	// which is used to store the sequence number and LSN of each page that has been written to the main log.
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

type Buffer struct {
	mu           sync.Mutex
	store        Store
	pages        map[disk.PageID]*disk.Page
	index        map[disk.PageID]entry
	nextSequence uint64
	nextShadow   int64
}

func NewBuffer(store Store) (*Buffer, error) {
	if store == nil {
		return nil, errors.New("copy-on-write disk adapter is required")
	}

	buf := &Buffer{
		store:        store,
		pages:        make(map[disk.PageID]*disk.Page),
		index:        make(map[disk.PageID]entry),
		nextSequence: 1,
	}
	if err := buf.RecoverAll(); err != nil {
		return nil, err
	}

	return buf, nil
}

func (b *Buffer) HasPage(id disk.PageID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, ok := b.pages[id]
	return ok
}

func (b *Buffer) ReadPage(id disk.PageID) (*disk.Page, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	if page, ok := b.pages[id]; ok {
		staged, err := writer.ClonePage(id, page)
		b.mu.Unlock()
		if err != nil {
			return nil, err
		}
		staged.MarkClean()
		return staged, nil
	}
	entry, ok := b.index[id]
	b.mu.Unlock()

	if !ok {
		return b.store.ReadPage(id)
	}

	shadowID := disk.NewPageID(ShadowFileName, entry.ShadowPageNum)
	page, err := b.store.ReadPage(shadowID)
	if err != nil {
		return nil, err
	}
	if recovery.Checksum(page.Data()) != entry.Checksum {
		return nil, fmt.Errorf("copy-on-write checksum mismatch for %q:%d", id.FileName(), id.PageNum())
	}

	current, err := writer.ClonePage(id, page)
	if err != nil {
		return nil, err
	}
	current.MarkClean()
	return current, nil
}

func (b *Buffer) WritePage(id disk.PageID, page *disk.Page) error {
	if err := b.stagePage(id, page); err != nil {
		return err
	}

	return b.flushPage(id)
}

func (b *Buffer) stagePage(id disk.PageID, page *disk.Page) error {
	if page == nil {
		return errors.New("page is required")
	}
	if err := id.Validate(); err != nil {
		return err
	}
	staged, err := writer.ClonePage(id, page)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.pages[id] = staged
	b.mu.Unlock()

	return nil
}

func (b *Buffer) flushPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	b.mu.Lock()
	page, ok := b.pages[id]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("page %q:%d is not staged", id.FileName(), id.PageNum())
	}
	sequence := b.nextSequence
	b.nextSequence++
	shadowPageNum := b.nextShadow
	b.nextShadow++
	b.mu.Unlock()

	shadowID := disk.NewPageID(ShadowFileName, shadowPageNum)
	shadowPage, err := writer.ClonePage(shadowID, page)
	if err != nil {
		return err
	}

	if err = b.store.WritePage(shadowID, shadowPage); err != nil {
		return err
	}

	if err = b.store.SyncFile(ShadowFileName); err != nil {
		return err
	}

	r := record{
		PageID: id,
		Entry: entry{
			Sequence:      sequence,
			ShadowPageNum: shadowPageNum,
			Checksum:      recovery.Checksum(page.Data()),
		},
	}
	encoded, err := r.Encode()
	if err != nil {
		return err
	}

	if _, err = b.store.AppendFile(ManifestFileName, encoded); err != nil {
		return err
	}

	if err = b.store.SyncFile(ManifestFileName); err != nil {
		return err
	}

	b.mu.Lock()
	b.index[id] = r.Entry
	delete(b.pages, id)
	b.mu.Unlock()

	return nil
}

func (b *Buffer) FlushAll() error {
	b.mu.Lock()
	ids := make([]disk.PageID, 0, len(b.pages))
	for id := range b.pages {
		ids = append(ids, id)
	}
	b.mu.Unlock()

	for _, id := range ids {
		if err := b.flushPage(id); err != nil {
			return err
		}
	}

	return nil
}

func (b *Buffer) RecoverPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	return b.RecoverAll()
}

func (b *Buffer) RecoverAll() error {
	index, nextShadow, nextSequence, err := b.loadManifestIndex()
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.index = index
	b.pages = make(map[disk.PageID]*disk.Page)
	b.nextShadow = nextShadow
	b.nextSequence = nextSequence
	b.mu.Unlock()

	return nil
}

func (b *Buffer) ResolvePage(id disk.PageID) (disk.PageID, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.index[id]
	if !ok {
		return disk.PageID{}, false
	}

	return disk.NewPageID(ShadowFileName, entry.ShadowPageNum), true
}

func (b *Buffer) loadManifestIndex() (map[disk.PageID]entry, int64, uint64, error) {
	size, err := b.store.FileSize(ManifestFileName)
	if err != nil {
		return nil, 0, 0, err
	}

	index := make(map[disk.PageID]entry)
	nextShadow := int64(0)
	nextSequence := uint64(1)
	for offset := int64(0); offset < size; {
		recordLen, ok, err := b.readManifestRecordLength(offset, size)
		if err != nil {
			return nil, 0, 0, err
		}
		if !ok {
			break
		}

		body := make([]byte, recordLen)
		n, err := b.store.ReadFileAt(ManifestFileName, offset+LengthFieldSize, body)
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

func (b *Buffer) readManifestRecordLength(offset, size int64) (uint32, bool, error) {
	lengthBuf := make([]byte, LengthFieldSize)
	n, err := b.store.ReadFileAt(ManifestFileName, offset, lengthBuf)
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
