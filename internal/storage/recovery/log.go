package recovery

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/tuanta7/cataraft/internal/storage/disk"
)

const (
	recordTypePageWrite uint8 = 1

	walLengthFieldSize   = 4
	walChecksumFieldSize = 4
	walRecordTypeSize    = 1
	walLSNFieldSize      = 8
	walFileNameLenSize   = 2
	walPageNumFieldSize  = 8
	walOffsetFieldSize   = 4
	walPayloadLenSize    = 4

	walHeaderSize        = walLengthFieldSize + walChecksumFieldSize
	walRecordPrefixSize  = walChecksumFieldSize + walRecordTypeSize + walLSNFieldSize + walFileNameLenSize + walPageNumFieldSize + walOffsetFieldSize + walPayloadLenSize
)

var walByteOrder = binary.BigEndian

type LogRecord struct {
	LSN      uint64
	PageID   disk.PageID
	Offset   uint32
	Payload  []byte
	Checksum uint32
}

type LogStore interface {
	AppendFile(fn string, data []byte) (int64, error)
	ReadFileAt(fn string, offset int64, buf []byte) (int, error)
	FileSize(fn string) (int64, error)
	SyncFile(fn string) error
}

type WriteAheadLog struct {
	mu         sync.Mutex
	store      LogStore
	fileName   string
	nextLSN    uint64
	flushedLSN uint64
	lastLSN    uint64
}

func NewWriteAheadLog(store LogStore, fileName string) (*WriteAheadLog, error) {
	if store == nil {
		return nil, errors.New("log store is required")
	}
	if fileName == "" {
		return nil, errors.New("wal file name is required")
	}

	wal := &WriteAheadLog{
		store:    store,
		fileName: fileName,
		nextLSN:  1,
	}

	records, err := wal.Records()
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		last := records[len(records)-1].LSN
		wal.lastLSN = last
		wal.flushedLSN = last
		wal.nextLSN = last + 1
	}

	return wal, nil
}

func (w *WriteAheadLog) FileName() string {
	return w.fileName
}

func (w *WriteAheadLog) FlushedLSN() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.flushedLSN
}

func (w *WriteAheadLog) LastLSN() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.lastLSN
}

func (w *WriteAheadLog) AppendPageWrite(page *disk.Page, offset int, payload []byte) (LogRecord, error) {
	if page == nil {
		return LogRecord{}, errors.New("page is required")
	}
	if offset < 0 {
		return LogRecord{}, fmt.Errorf("invalid record offset %d", offset)
	}
	if offset > math.MaxUint32 {
		return LogRecord{}, fmt.Errorf("record offset %d exceeds uint32", offset)
	}
	if err := page.ID().Validate(); err != nil {
		return LogRecord{}, err
	}

	w.mu.Lock()
	record := LogRecord{
		LSN:     w.nextLSN,
		PageID:  page.ID(),
		Offset:  uint32(offset),
		Payload: bytes.Clone(payload),
	}
	encoded, err := encodeLogRecord(record)
	if err != nil {
		w.mu.Unlock()
		return LogRecord{}, err
	}

	if _, err := w.store.AppendFile(w.fileName, encoded); err != nil {
		w.mu.Unlock()
		return LogRecord{}, err
	}

	record.Checksum = checksum(encoded[8:])
	w.lastLSN = record.LSN
	w.nextLSN++
	w.mu.Unlock()

	if err := Apply(record, page); err != nil {
		return LogRecord{}, err
	}

	return record, nil
}

func (w *WriteAheadLog) Flush() error {
	w.mu.Lock()
	lastLSN := w.lastLSN
	w.mu.Unlock()

	if err := w.store.SyncFile(w.fileName); err != nil {
		return err
	}

	w.mu.Lock()
	if lastLSN > w.flushedLSN {
		w.flushedLSN = lastLSN
	}
	w.mu.Unlock()

	return nil
}

func (w *WriteAheadLog) FlushThrough(lsn uint64) error {
	if err := w.store.SyncFile(w.fileName); err != nil {
		return err
	}

	w.mu.Lock()
	if lsn > w.lastLSN {
		lsn = w.lastLSN
	}
	if lsn > w.flushedLSN {
		w.flushedLSN = lsn
	}
	w.mu.Unlock()

	return nil
}

func (w *WriteAheadLog) Records() ([]LogRecord, error) {
	size, err := w.store.FileSize(w.fileName)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	records := make([]LogRecord, 0)
	for offset := int64(0); offset < size; {
		lengthBuf := make([]byte, walLengthFieldSize)
		n, err := w.store.ReadFileAt(w.fileName, offset, lengthBuf)
		if err != nil {
			if errors.Is(err, io.EOF) && n == 0 {
				break
			}
			return nil, err
		}
		if n != len(lengthBuf) {
			return nil, io.ErrUnexpectedEOF
		}

		recordLen := walByteOrder.Uint32(lengthBuf)
		if recordLen == 0 {
			return nil, errors.New("invalid zero-length wal record")
		}

		body := make([]byte, recordLen)
		n, err = w.store.ReadFileAt(w.fileName, offset+walLengthFieldSize, body)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		if n != len(body) {
			return nil, io.ErrUnexpectedEOF
		}

		record, err := decodeLogRecord(body)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
		offset += walLengthFieldSize + int64(recordLen)
	}

	return records, nil
}

func (w *WriteAheadLog) Replay(apply func(LogRecord) error) error {
	records, err := w.Records()
	if err != nil {
		return err
	}

	for _, record := range records {
		if err := apply(record); err != nil {
			return err
		}
	}

	return nil
}

func Apply(record LogRecord, page *disk.Page) error {
	if page == nil {
		return errors.New("page is required")
	}
	if record.PageID != page.ID() {
		return fmt.Errorf("record page %q:%d does not match page %q:%d",
			record.PageID.FileName(), record.PageID.PageNum(),
			page.ID().FileName(), page.ID().PageNum(),
		)
	}
	if page.LSN() >= record.LSN {
		return nil
	}
	if err := page.WriteAt(int(record.Offset), record.Payload); err != nil {
		return err
	}

	page.SetLSN(record.LSN)
	return nil
}

func encodeLogRecord(record LogRecord) ([]byte, error) {
	if err := record.PageID.Validate(); err != nil {
		return nil, err
	}
	if record.Offset > math.MaxInt32 && int64(record.Offset)+int64(len(record.Payload)) > math.MaxInt32 {
		return nil, errors.New("record offset overflow")
	}
	if int(record.Offset)+len(record.Payload) > math.MaxInt32 {
		return nil, errors.New("record offset overflow")
	}
	if len(record.Payload) == 0 {
		return nil, errors.New("record payload is required")
	}
	if len(record.PageID.FileName()) > math.MaxUint16 {
		return nil, errors.New("file name too long")
	}

	bodyLen := walRecordPrefixSize + len(record.PageID.FileName()) + len(record.Payload)
	buf := bytes.NewBuffer(make([]byte, 0, walLengthFieldSize+bodyLen))
	if err := binary.Write(buf, walByteOrder, uint32(bodyLen)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, uint32(0)); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(recordTypePageWrite); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, record.LSN); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, uint16(len(record.PageID.FileName()))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(record.PageID.FileName()); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, uint64(record.PageID.PageNum())); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, record.Offset); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, walByteOrder, uint32(len(record.Payload))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(record.Payload); err != nil {
		return nil, err
	}

	encoded := buf.Bytes()
	walByteOrder.PutUint32(
		encoded[walLengthFieldSize:walHeaderSize],
		checksum(encoded[walHeaderSize:]),
	)
	return encoded, nil
}

func decodeLogRecord(body []byte) (LogRecord, error) {
	if len(body) < walRecordPrefixSize {
		return LogRecord{}, io.ErrUnexpectedEOF
	}

	expectedChecksum := walByteOrder.Uint32(body[:walChecksumFieldSize])
	if checksum(body[walChecksumFieldSize:]) != expectedChecksum {
		return LogRecord{}, errors.New("wal checksum mismatch")
	}

	reader := bytes.NewReader(body[walChecksumFieldSize:])

	recordType, err := reader.ReadByte()
	if err != nil {
		return LogRecord{}, err
	}
	if recordType != recordTypePageWrite {
		return LogRecord{}, fmt.Errorf("unsupported wal record type %d", recordType)
	}

	var record LogRecord
	record.Checksum = expectedChecksum
	if err := binary.Read(reader, walByteOrder, &record.LSN); err != nil {
		return LogRecord{}, err
	}

	var fileNameLen uint16
	if err := binary.Read(reader, walByteOrder, &fileNameLen); err != nil {
		return LogRecord{}, err
	}

	fileName := make([]byte, fileNameLen)
	if _, err := io.ReadFull(reader, fileName); err != nil {
		return LogRecord{}, err
	}

	var pageNum uint64
	if err := binary.Read(reader, walByteOrder, &pageNum); err != nil {
		return LogRecord{}, err
	}
	if pageNum > math.MaxInt64 {
		return LogRecord{}, errors.New("page number overflow")
	}

	if err := binary.Read(reader, walByteOrder, &record.Offset); err != nil {
		return LogRecord{}, err
	}

	var payloadLen uint32
	if err := binary.Read(reader, walByteOrder, &payloadLen); err != nil {
		return LogRecord{}, err
	}

	record.Payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, record.Payload); err != nil {
		return LogRecord{}, err
	}
	if reader.Len() != 0 {
		return LogRecord{}, errors.New("trailing wal record data")
	}

	record.PageID = disk.NewPageID(string(fileName), int64(pageNum))
	if err := record.PageID.Validate(); err != nil {
		return LogRecord{}, err
	}

	return record, nil
}
