package cow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/recovery"
	"github.com/tuanta7/cataraft/internal/storage/writer"
)

const (
	// DefaultCopyOnWriteShadowFileName is the default file name for the copy-on-write shadow pages,
	// which are used to store the current state of the pages before they are written to the main log.
	DefaultCopyOnWriteShadowFileName = "copyonwrite.pages"

	// DefaultCopyOnWriteManifestFileName is the default file name for the copy-on-write manifest,
	// which is used to store the sequence number and LSN of each page that has been written to the main log.
	DefaultCopyOnWriteManifestFileName = "copyonwrite.manifest"

	copyOnWriteRecordTypePageVersion uint8 = 1

	copyOnWriteLengthFieldSize    = 4
	copyOnWriteChecksumFieldSize  = 4
	copyOnWriteRecordTypeSize     = 1
	copyOnWriteSequenceSize       = 8
	copyOnWriteLSNSize            = 8
	copyOnWriteShadowPageNumSize  = 8
	copyOnWritePageChecksumSize   = 4
	copyOnWriteFileNameLenSize    = 2
	copyOnWriteLogicalPageNumSize = 8

	copyOnWriteHeaderSize       = copyOnWriteLengthFieldSize + copyOnWriteChecksumFieldSize
	copyOnWriteRecordPrefixSize = copyOnWriteChecksumFieldSize +
		copyOnWriteRecordTypeSize +
		copyOnWriteSequenceSize +
		copyOnWriteLSNSize +
		copyOnWriteShadowPageNumSize +
		copyOnWritePageChecksumSize +
		copyOnWriteFileNameLenSize +
		copyOnWriteLogicalPageNumSize
)

type CopyOnWriteStore interface {
	ReadPage(id disk.PageID) (*disk.Page, error)
	WritePage(id disk.PageID, page *disk.Page) error
	SyncFile(fn string) error
	AppendFile(fn string, data []byte) (int64, error)
	ReadFileAt(fn string, offset int64, buf []byte) (int, error)
	FileSize(fn string) (int64, error)
}

type CopyOnWriteBuffer struct {
	mu           sync.Mutex
	store        CopyOnWriteStore
	shadowFile   string
	manifestFile string
	pages        map[disk.PageID]*disk.Page
	index        map[disk.PageID]copyOnWriteEntry
	nextSequence uint64
	nextShadow   int64
}

type copyOnWriteEntry struct {
	Sequence      uint64
	ShadowPageNum int64
	LSN           uint64
	Checksum      uint32
}

type copyOnWriteRecord struct {
	PageID disk.PageID
	Entry  copyOnWriteEntry
}

func NewCopyOnWriteBuffer(store CopyOnWriteStore, shadowFile, manifestFile string) (*CopyOnWriteBuffer, error) {
	if store == nil {
		return nil, errors.New("copy-on-write diskAdapter is required")
	}
	if shadowFile == "" {
		shadowFile = DefaultCopyOnWriteShadowFileName
	}
	if manifestFile == "" {
		manifestFile = DefaultCopyOnWriteManifestFileName
	}

	buf := &CopyOnWriteBuffer{
		store:        store,
		shadowFile:   shadowFile,
		manifestFile: manifestFile,
		pages:        make(map[disk.PageID]*disk.Page),
		index:        make(map[disk.PageID]copyOnWriteEntry),
		nextSequence: 1,
	}
	if err := buf.RecoverAll(); err != nil {
		return nil, err
	}

	return buf, nil
}

func (b *CopyOnWriteBuffer) ShadowFile() string {
	return b.shadowFile
}

func (b *CopyOnWriteBuffer) ManifestFile() string {
	return b.manifestFile
}

func (b *CopyOnWriteBuffer) StagePage(id disk.PageID, page *disk.Page) error {
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

func (b *CopyOnWriteBuffer) HasPage(id disk.PageID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, ok := b.pages[id]
	return ok
}

func (b *CopyOnWriteBuffer) ReadPage(id disk.PageID) (*disk.Page, error) {
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

	shadowID := disk.NewPageID(b.shadowFile, entry.ShadowPageNum)
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
	current.SetLSN(entry.LSN)
	current.MarkClean()
	return current, nil
}

func (b *CopyOnWriteBuffer) WritePage(id disk.PageID, page *disk.Page) error {
	if err := b.StagePage(id, page); err != nil {
		return err
	}

	return b.FlushPage(id)
}

func (b *CopyOnWriteBuffer) FlushPage(id disk.PageID) error {
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

	shadowID := disk.NewPageID(b.shadowFile, shadowPageNum)
	shadowPage, err := writer.ClonePage(shadowID, page)
	if err != nil {
		return err
	}
	if err := b.store.WritePage(shadowID, shadowPage); err != nil {
		return err
	}
	if err := b.store.SyncFile(b.shadowFile); err != nil {
		return err
	}

	record := copyOnWriteRecord{
		PageID: id,
		Entry: copyOnWriteEntry{
			Sequence:      sequence,
			ShadowPageNum: shadowPageNum,
			LSN:           page.LSN(),
			Checksum:      recovery.Checksum(page.Data()),
		},
	}
	encoded, err := encodeCopyOnWriteRecord(record)
	if err != nil {
		return err
	}
	if _, err := b.store.AppendFile(b.manifestFile, encoded); err != nil {
		return err
	}
	if err := b.store.SyncFile(b.manifestFile); err != nil {
		return err
	}

	b.mu.Lock()
	b.index[id] = record.Entry
	delete(b.pages, id)
	b.mu.Unlock()

	return nil
}

func (b *CopyOnWriteBuffer) FlushAll() error {
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

func (b *CopyOnWriteBuffer) RecoverPage(id disk.PageID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	return b.RecoverAll()
}

func (b *CopyOnWriteBuffer) RecoverAll() error {
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

func (b *CopyOnWriteBuffer) ResolvePage(id disk.PageID) (disk.PageID, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.index[id]
	if !ok {
		return disk.PageID{}, false
	}

	return disk.NewPageID(b.shadowFile, entry.ShadowPageNum), true
}

func (b *CopyOnWriteBuffer) loadManifestIndex() (map[disk.PageID]copyOnWriteEntry, int64, uint64, error) {
	size, err := b.store.FileSize(b.manifestFile)
	if err != nil {
		return nil, 0, 0, err
	}

	index := make(map[disk.PageID]copyOnWriteEntry)
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
		n, err := b.store.ReadFileAt(b.manifestFile, offset+copyOnWriteLengthFieldSize, body)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, 0, 0, err
		}
		if n != len(body) {
			break
		}

		record, err := decodeCopyOnWriteRecord(body)
		if err != nil {
			return nil, 0, 0, err
		}
		current, ok := index[record.PageID]
		if !ok || record.Entry.Sequence > current.Sequence {
			index[record.PageID] = record.Entry
		}
		if record.Entry.ShadowPageNum >= nextShadow {
			nextShadow = record.Entry.ShadowPageNum + 1
		}
		if record.Entry.Sequence >= nextSequence {
			nextSequence = record.Entry.Sequence + 1
		}

		offset += copyOnWriteLengthFieldSize + int64(recordLen)
	}

	return index, nextShadow, nextSequence, nil
}

func (b *CopyOnWriteBuffer) readManifestRecordLength(offset, size int64) (uint32, bool, error) {
	lengthBuf := make([]byte, copyOnWriteLengthFieldSize)
	n, err := b.store.ReadFileAt(b.manifestFile, offset, lengthBuf)
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
	if offset+copyOnWriteLengthFieldSize+int64(recordLen) > size {
		return 0, false, nil
	}

	return recordLen, true, nil
}

func encodeCopyOnWriteRecord(record copyOnWriteRecord) ([]byte, error) {
	if err := record.PageID.Validate(); err != nil {
		return nil, err
	}
	if record.Entry.ShadowPageNum < 0 {
		return nil, errors.New("shadow page number must be non-negative")
	}
	if len(record.PageID.FileName()) > math.MaxUint16 {
		return nil, errors.New("file name too long for copy-on-write record")
	}

	bodyLen := copyOnWriteRecordPrefixSize + len(record.PageID.FileName())
	buf := bytes.NewBuffer(make([]byte, 0, copyOnWriteLengthFieldSize+bodyLen))
	if err := binary.Write(buf, config.ByteOrder, uint32(bodyLen)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint32(0)); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(copyOnWriteRecordTypePageVersion); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, record.Entry.Sequence); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, record.Entry.LSN); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint64(record.Entry.ShadowPageNum)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, record.Entry.Checksum); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint16(len(record.PageID.FileName()))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(record.PageID.FileName()); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint64(record.PageID.PageNum())); err != nil {
		return nil, err
	}

	encoded := buf.Bytes()
	config.ByteOrder.PutUint32(
		encoded[copyOnWriteLengthFieldSize:copyOnWriteHeaderSize],
		recovery.Checksum(encoded[copyOnWriteHeaderSize:]),
	)
	return encoded, nil
}

func decodeCopyOnWriteRecord(body []byte) (copyOnWriteRecord, error) {
	if len(body) < copyOnWriteRecordPrefixSize {
		return copyOnWriteRecord{}, io.ErrUnexpectedEOF
	}

	expectedChecksum := config.ByteOrder.Uint32(body[:copyOnWriteChecksumFieldSize])
	if recovery.Checksum(body[copyOnWriteChecksumFieldSize:]) != expectedChecksum {
		return copyOnWriteRecord{}, errors.New("copy-on-write manifest checksum mismatch")
	}

	reader := bytes.NewReader(body[copyOnWriteChecksumFieldSize:])

	recordType, err := reader.ReadByte()
	if err != nil {
		return copyOnWriteRecord{}, err
	}
	if recordType != copyOnWriteRecordTypePageVersion {
		return copyOnWriteRecord{}, fmt.Errorf("unsupported copy-on-write record type %d", recordType)
	}

	var record copyOnWriteRecord
	if err := binary.Read(reader, config.ByteOrder, &record.Entry.Sequence); err != nil {
		return copyOnWriteRecord{}, err
	}
	if err := binary.Read(reader, config.ByteOrder, &record.Entry.LSN); err != nil {
		return copyOnWriteRecord{}, err
	}
	var shadowPageNum uint64
	if err := binary.Read(reader, config.ByteOrder, &shadowPageNum); err != nil {
		return copyOnWriteRecord{}, err
	}
	if shadowPageNum > math.MaxInt64 {
		return copyOnWriteRecord{}, errors.New("copy-on-write shadow page number exceeds int64")
	}
	record.Entry.ShadowPageNum = int64(shadowPageNum)
	if err := binary.Read(reader, config.ByteOrder, &record.Entry.Checksum); err != nil {
		return copyOnWriteRecord{}, err
	}
	var fileNameLen uint16
	if err := binary.Read(reader, config.ByteOrder, &fileNameLen); err != nil {
		return copyOnWriteRecord{}, err
	}
	fileName := make([]byte, fileNameLen)
	if _, err := io.ReadFull(reader, fileName); err != nil {
		return copyOnWriteRecord{}, err
	}
	var logicalPageNum uint64
	if err := binary.Read(reader, config.ByteOrder, &logicalPageNum); err != nil {
		return copyOnWriteRecord{}, err
	}
	if logicalPageNum > math.MaxInt64 {
		return copyOnWriteRecord{}, errors.New("copy-on-write logical page number exceeds int64")
	}

	record.PageID = disk.NewPageID(string(fileName), int64(logicalPageNum))
	if err := record.PageID.Validate(); err != nil {
		return copyOnWriteRecord{}, err
	}

	return record, nil
}
