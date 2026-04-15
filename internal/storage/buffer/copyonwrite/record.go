package copyonwrite

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type entry struct {
	Sequence      uint64
	ShadowPageNum int64
	Checksum      uint32
}

type record struct {
	PageID disk.PageID
	Entry  entry
}

func (r *record) Decode(body []byte) error {
	if len(body) < RecordPrefixSize {
		return io.ErrUnexpectedEOF
	}

	expectedChecksum := config.ByteOrder.Uint32(body[:ChecksumFieldSize])
	if checksum(body[ChecksumFieldSize:]) != expectedChecksum {
		return errors.New("copy-on-write manifest checksum mismatch")
	}

	reader := bytes.NewReader(body[ChecksumFieldSize:])

	recordType, err := reader.ReadByte()
	if err != nil {
		return err
	}

	if recordType != RecordTypePageVersion {
		return fmt.Errorf("unsupported copy-on-write record type %d", recordType)
	}

	if err := binary.Read(reader, config.ByteOrder, &r.Entry.Sequence); err != nil {
		return err
	}

	var shadowPageNum uint64
	if err := binary.Read(reader, config.ByteOrder, &shadowPageNum); err != nil {
		return err
	}

	if shadowPageNum > math.MaxInt64 {
		return errors.New("copy-on-write shadow page number exceeds int64")
	}

	r.Entry.ShadowPageNum = int64(shadowPageNum)

	if err := binary.Read(reader, config.ByteOrder, &r.Entry.Checksum); err != nil {
		return err
	}

	var fileNameLen uint16
	if err := binary.Read(reader, config.ByteOrder, &fileNameLen); err != nil {
		return err
	}

	fileName := make([]byte, fileNameLen)
	if _, err := io.ReadFull(reader, fileName); err != nil {
		return err
	}

	var logicalPageNum uint64
	if err := binary.Read(reader, config.ByteOrder, &logicalPageNum); err != nil {
		return err
	}
	
	if logicalPageNum > math.MaxInt64 {
		return errors.New("copy-on-write logical page number exceeds int64")
	}

	r.PageID = disk.NewPageID(string(fileName), int64(logicalPageNum))
	if err := r.PageID.Validate(); err != nil {
		return err
	}

	return nil
}

func (r *record) Encode() ([]byte, error) {
	if err := r.PageID.Validate(); err != nil {
		return nil, err
	}
	if r.Entry.ShadowPageNum < 0 {
		return nil, errors.New("shadow page number must be non-negative")
	}
	if len(r.PageID.FileName()) > math.MaxUint16 {
		return nil, errors.New("file name too long for copy-on-write record")
	}

	bodyLen := RecordPrefixSize + len(r.PageID.FileName())
	buf := bytes.NewBuffer(make([]byte, 0, LengthFieldSize+bodyLen))
	if err := binary.Write(buf, config.ByteOrder, uint32(bodyLen)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint32(0)); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(RecordTypePageVersion); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, r.Entry.Sequence); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint64(r.Entry.ShadowPageNum)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, r.Entry.Checksum); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint16(len(r.PageID.FileName()))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(r.PageID.FileName()); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, config.ByteOrder, uint64(r.PageID.PageNum())); err != nil {
		return nil, err
	}

	encoded := buf.Bytes()
	config.ByteOrder.PutUint32(
		encoded[LengthFieldSize:HeaderSize],
		checksum(encoded[HeaderSize:]),
	)
	return encoded, nil
}
