package dwb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/recovery"
	"github.com/tuanta7/cataraft/internal/storage/writer"
)

const (
	DefaultDoubleWriteFileName = "doublewrite.db"
	DoubleWriteSlots           = 128

	doubleWriteMagic      = "CATADWB1"
	doubleWriteMetaPages  = 2
	doubleWriteActiveFlag = 1

	doubleWriteMagicSize        = 8
	doubleWriteActiveSize       = 1
	doubleWriteReservedSize     = 7
	doubleWriteSequenceSize     = 8
	doubleWritePageNumSize      = 8
	doubleWriteLSNSize          = 8
	doubleWriteChecksumSize     = 4
	doubleWriteFileNameLenSize  = 2
	doubleWriteMetadataBaseSize = doubleWriteMagicSize + doubleWriteActiveSize + doubleWriteReservedSize + doubleWriteSequenceSize + doubleWritePageNumSize + doubleWriteLSNSize + doubleWriteChecksumSize + doubleWriteFileNameLenSize
)

type DoubleWriteStore interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, page *disk.Page) error
	SyncFile(fn string) error
}

type WALFlusher interface {
	FlushedLSN() uint64
	FlushThrough(lsn uint64) error
}

type DoubleWriteBuffer struct {
	mu          sync.Mutex
	store       DoubleWriteStore
	stagingFile string
	pages       map[disk.PageID]*disk.Page
	assignments map[disk.PageID]int
	occupied    [DoubleWriteSlots]bool
	nextSeq     uint64
}

type doubleWriteMetadata struct {
	Active   bool
	Sequence uint64
	PageID   disk.PageID
	LSN      uint64
	Checksum uint32
}

type doubleWriteEntry struct {
	slot int
	meta doubleWriteMetadata
	data *disk.Page
}

func NewDoubleWriteBufferWithWAL(store DoubleWriteStore, stagingFile string, wal WALFlusher) (*DoubleWriteBuffer, error) {
	_ = wal
	return NewDoubleWriteBuffer(store, stagingFile)
}

func NewDoubleWriteBuffer(store DoubleWriteStore, stagingFile string) (*DoubleWriteBuffer, error) {
	if store == nil {
		return nil, errors.New("double-write diskAdapter is required")
	}
	if stagingFile == "" {
		stagingFile = DefaultDoubleWriteFileName
	}

	return &DoubleWriteBuffer{
		store:       store,
		stagingFile: stagingFile,
		pages:       make(map[disk.PageID]*disk.Page),
		assignments: make(map[disk.PageID]int),
		nextSeq:     1,
	}, nil
}

func (b *DoubleWriteBuffer) StagingFile() string {
	return b.stagingFile
}

func (b *DoubleWriteBuffer) StagePage(id disk.PageID, page *disk.Page) error {
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
	defer b.mu.Unlock()

	if _, ok := b.assignments[id]; !ok {
		slot, err := b.allocateSlot()
		if err != nil {
			return err
		}
		b.assignments[id] = slot
	}
	b.pages[id] = staged

	return nil
}

func (b *DoubleWriteBuffer) HasPage(id disk.PageID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, ok := b.pages[id]
	return ok
}

func (b *DoubleWriteBuffer) FlushPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	b.mu.Lock()
	page, ok := b.pages[id]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("page %q:%d is not staged", id.FileName(), id.PageNum())
	}
	slot, ok := b.assignments[id]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("page %q:%d has no double-write slot", id.FileName(), id.PageNum())
	}
	sequence := b.nextSeq
	b.nextSeq++
	b.mu.Unlock()

	metaID, dataID := b.slotPageIDs(slot)
	metaPage, err := b.newMetadataPage(metaID, doubleWriteMetadata{
		Active:   true,
		Sequence: sequence,
		PageID:   id,
		LSN:      page.LSN(),
		Checksum: recovery.Checksum(page.Data()),
	})
	if err != nil {
		return err
	}
	dataPage, err := writer.ClonePage(dataID, page)
	if err != nil {
		return err
	}

	if err := b.store.WritePage(metaID, metaPage); err != nil {
		return err
	}
	if err := b.store.WritePage(dataID, dataPage); err != nil {
		return err
	}
	if err := b.store.SyncFile(b.stagingFile); err != nil {
		return err
	}

	if err := b.store.WritePage(id, page); err != nil {
		return err
	}
	if err := b.store.SyncFile(id.FileName()); err != nil {
		return err
	}

	clearMeta, err := b.newMetadataPage(metaID, doubleWriteMetadata{})
	if err != nil {
		return err
	}
	if err := b.store.WritePage(metaID, clearMeta); err != nil {
		return err
	}
	if err := b.store.SyncFile(b.stagingFile); err != nil {
		return err
	}

	b.mu.Lock()
	delete(b.pages, id)
	delete(b.assignments, id)
	b.occupied[slot] = false
	b.mu.Unlock()

	return nil
}

func (b *DoubleWriteBuffer) FlushAll() error {
	b.mu.Lock()
	ids := make([]disk.PageID, 0, len(b.pages))
	for id := range b.pages {
		ids = append(ids, id)
	}
	b.mu.Unlock()

	for _, id := range ids {
		if err := b.FlushPage(id); err != nil {
			return err
		}
	}

	return nil
}

func (b *DoubleWriteBuffer) RecoverPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	entries, err := b.scanEntries()
	if err != nil {
		return err
	}
	var latest doubleWriteEntry
	found := false
	slotsToClear := make([]int, 0)
	for _, entry := range entries {
		if entry.meta.PageID != id {
			continue
		}
		slotsToClear = append(slotsToClear, entry.slot)
		if !found || entry.meta.Sequence > latest.meta.Sequence {
			latest = entry
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no double-write entry found for %q:%d", id.FileName(), id.PageNum())
	}

	if err := b.restoreEntry(latest); err != nil {
		return err
	}
	return b.clearSlots(slotsToClear...)
}

func (b *DoubleWriteBuffer) RecoverAll() error {
	entries, err := b.scanEntries()
	if err != nil {
		return err
	}

	latest := make(map[disk.PageID]doubleWriteEntry)
	for _, entry := range entries {
		current, ok := latest[entry.meta.PageID]
		if !ok || entry.meta.Sequence > current.meta.Sequence {
			latest[entry.meta.PageID] = entry
		}
	}

	for _, entry := range latest {
		if err := b.restoreEntry(entry); err != nil {
			return err
		}
	}

	slotsToClear := make([]int, 0, len(entries))
	for _, entry := range entries {
		slotsToClear = append(slotsToClear, entry.slot)
	}
	return b.clearSlots(slotsToClear...)
}

func (b *DoubleWriteBuffer) scanEntries() ([]doubleWriteEntry, error) {
	entries := make([]doubleWriteEntry, 0)
	for slot := 0; slot < DoubleWriteSlots; slot++ {
		metaID, dataID := b.slotPageIDs(slot)
		metaPage, err := b.store.ReadPage(metaID)
		if err != nil {
			return nil, err
		}

		meta, err := decodeDoubleWriteMetadata(metaPage)
		if err != nil {
			return nil, err
		}
		if !meta.Active {
			continue
		}

		dataPage, err := b.store.ReadPage(dataID)
		if err != nil {
			return nil, err
		}
		if recovery.Checksum(dataPage.Data()) != meta.Checksum {
			return nil, fmt.Errorf("double-write checksum mismatch for slot %d", slot)
		}
		dataPage.SetLSN(meta.LSN)

		entries = append(entries, doubleWriteEntry{
			slot: slot,
			meta: meta,
			data: dataPage,
		})
	}

	return entries, nil
}

func (b *DoubleWriteBuffer) restoreEntry(entry doubleWriteEntry) error {
	restorePage, err := writer.ClonePage(entry.meta.PageID, entry.data)
	if err != nil {
		return err
	}
	restorePage.SetLSN(entry.meta.LSN)

	if err := b.store.WritePage(entry.meta.PageID, restorePage); err != nil {
		return err
	}
	if err := b.store.SyncFile(entry.meta.PageID.FileName()); err != nil {
		return err
	}

	return nil
}

func (b *DoubleWriteBuffer) clearSlots(slots ...int) error {
	seen := make(map[int]struct{}, len(slots))
	for _, slot := range slots {
		if _, ok := seen[slot]; ok {
			continue
		}
		seen[slot] = struct{}{}

		metaID, _ := b.slotPageIDs(slot)
		clearMeta, err := b.newMetadataPage(metaID, doubleWriteMetadata{})
		if err != nil {
			return err
		}
		if err := b.store.WritePage(metaID, clearMeta); err != nil {
			return err
		}
	}
	if len(seen) == 0 {
		return nil
	}
	if err := b.store.SyncFile(b.stagingFile); err != nil {
		return err
	}

	return nil
}

func (b *DoubleWriteBuffer) StagedPageIDs() []disk.PageID {
	b.mu.Lock()
	defer b.mu.Unlock()

	ids := make([]disk.PageID, 0, len(b.pages))
	for id := range b.pages {
		ids = append(ids, id)
	}
	return ids
}

func (b *DoubleWriteBuffer) allocateSlot() (int, error) {
	for slot := 0; slot < DoubleWriteSlots; slot++ {
		if b.occupied[slot] {
			continue
		}
		b.occupied[slot] = true
		return slot, nil
	}

	return 0, errors.New("double-write buffer is full")
}

func (b *DoubleWriteBuffer) slotPageIDs(slot int) (disk.PageID, disk.PageID) {
	base := int64(slot * doubleWriteMetaPages)
	return disk.NewPageID(b.stagingFile, base), disk.NewPageID(b.stagingFile, base+1)
}

func (b *DoubleWriteBuffer) newMetadataPage(id disk.PageID, meta doubleWriteMetadata) (*disk.Page, error) {
	page := disk.NewPage(id)
	buf, err := encodeDoubleWriteMetadata(meta)
	if err != nil {
		return nil, err
	}
	if err := page.Reset(buf); err != nil {
		return nil, err
	}
	return page, nil
}

func encodeDoubleWriteMetadata(meta doubleWriteMetadata) ([]byte, error) {
	fileName := meta.PageID.FileName()
	if len(fileName) > int(^uint16(0)) {
		return nil, errors.New("file name too long for double-write metadata")
	}
	if doubleWriteMetadataBaseSize+len(fileName) > len(disk.NewPage(disk.NewPageID("meta", 0)).Data()) {
		return nil, errors.New("double-write metadata exceeds page size")
	}

	buf := bytes.NewBuffer(make([]byte, 0, doubleWriteMetadataBaseSize+len(fileName)))
	if _, err := buf.WriteString(doubleWriteMagic); err != nil {
		return nil, err
	}
	active := byte(0)
	if meta.Active {
		active = doubleWriteActiveFlag
	}
	if err := buf.WriteByte(active); err != nil {
		return nil, err
	}
	if _, err := buf.Write(make([]byte, doubleWriteReservedSize)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, meta.Sequence); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint64(meta.PageID.PageNum())); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, meta.LSN); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, meta.Checksum); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint16(len(fileName))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(fileName); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decodeDoubleWriteMetadata(page *disk.Page) (doubleWriteMetadata, error) {
	if page == nil {
		return doubleWriteMetadata{}, errors.New("metadata page is required")
	}

	raw := page.Data()
	if len(raw) < doubleWriteMetadataBaseSize {
		return doubleWriteMetadata{}, errors.New("double-write metadata page is too small")
	}
	if string(raw[:doubleWriteMagicSize]) != doubleWriteMagic {
		return doubleWriteMetadata{}, nil
	}

	meta := doubleWriteMetadata{
		Active: raw[doubleWriteMagicSize] == doubleWriteActiveFlag,
	}
	offset := doubleWriteMagicSize + doubleWriteActiveSize + doubleWriteReservedSize
	meta.Sequence = config.ByteOrder.Uint64(raw[offset : offset+doubleWriteSequenceSize])
	offset += doubleWriteSequenceSize
	meta.PageID = disk.NewPageID("", int64(config.ByteOrder.Uint64(raw[offset:offset+doubleWritePageNumSize])))
	offset += doubleWritePageNumSize
	meta.LSN = config.ByteOrder.Uint64(raw[offset : offset+doubleWriteLSNSize])
	offset += doubleWriteLSNSize
	meta.Checksum = config.ByteOrder.Uint32(raw[offset : offset+doubleWriteChecksumSize])
	offset += doubleWriteChecksumSize
	fileNameLen := int(config.ByteOrder.Uint16(raw[offset : offset+doubleWriteFileNameLenSize]))
	offset += doubleWriteFileNameLenSize
	if offset+fileNameLen > len(raw) {
		return doubleWriteMetadata{}, errors.New("double-write metadata file name exceeds page boundary")
	}
	meta.PageID = disk.NewPageID(string(raw[offset:offset+fileNameLen]), meta.PageID.PageNum())

	if !meta.Active {
		return meta, nil
	}
	if err := meta.PageID.Validate(); err != nil {
		return doubleWriteMetadata{}, err
	}

	return meta, nil
}
