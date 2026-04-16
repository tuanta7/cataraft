package copyonwrite

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	mock "github.com/tuanta7/cataraft/mocks"
	"go.uber.org/mock/gomock"
)

type CopyOnWriteBufferTestSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *mock.CopyOnWriteStore
}

func TestCopyOnWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(CopyOnWriteBufferTestSuite))
}

func (s *CopyOnWriteBufferTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = mock.NewCopyOnWriteStore(s.ctrl)
}

func (s *CopyOnWriteBufferTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CopyOnWriteBufferTestSuite) TestStagePageKeepsLocalCopy() {
	buf := s.newBuffer()

	id := disk.NewPageID("table/main.db", 1)
	page := disk.NewPage(id)
	s.Require().NoError(page.Write([]byte("stage")))

	s.Require().NoError(buf.stagePage(id, page))
}

func (s *CopyOnWriteBufferTestSuite) TestFlushPageWritesShadowThenAppendsManifest() {
	buf := s.newBuffer()

	id := disk.NewPageID("table/main.db", 2)
	page := disk.NewPage(id)
	s.Require().NoError(page.Write([]byte("flush")))
	s.Require().NoError(buf.stagePage(id, page))

	shadowID := disk.NewPageID("copyonwrite.pages", 0)
	var manifestRecord []byte

	gomock.InOrder(
		s.store.EXPECT().WritePage(shadowID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile("copyonwrite.pages").Return(nil),
		s.store.EXPECT().AppendFile("copyonwrite.manifest", gomock.Any()).DoAndReturn(func(_ string, data []byte) (int64, error) {
			manifestRecord = bytes.Clone(data)
			return 0, nil
		}),
		s.store.EXPECT().SyncFile("copyonwrite.manifest").Return(nil),
	)

	s.Require().NoError(buf.flushPage(id))

	resolvedID, ok := buf.ResolvePage(id)
	s.True(ok)
	s.Equal(shadowID, resolvedID)

	record := decodeCopyOnWriteRecordForTest(manifestRecord)
	s.Equal(id, record.pageID)
	s.Equal(int64(0), record.shadowPageNum)
}

func (s *CopyOnWriteBufferTestSuite) TestReadPageUsesLatestRecoveredShadowPage() {
	buf := s.newBuffer()

	id := disk.NewPageID("table/main.db", 7)
	recordOld := encodeCopyOnWriteRecordForTest(id, 1, 2, []byte("old"))
	recordNew := encodeCopyOnWriteRecordForTest(id, 2, 5, []byte("newer"))
	manifest := append(recordOld, recordNew...)

	s.expectManifestReads(manifest)
	s.Require().NoError(buf.RecoverAll())

	shadowID, ok := buf.ResolvePage(id)
	s.True(ok)
	s.Equal(disk.NewPageID("copyonwrite.pages", 5), shadowID)

	shadowPage := disk.NewPage(shadowID)
	s.Require().NoError(shadowPage.Write([]byte("newer")))
	shadowPage.MarkClean()

	s.store.EXPECT().ReadPage(shadowID).Return(shadowPage, nil)

	page, err := buf.ReadPage(id)
	s.Require().NoError(err)
	s.Equal(id, page.ID())
	s.False(page.IsDirty())
	s.Equal([]byte("newer"), page.Data()[:5])
}

func (s *CopyOnWriteBufferTestSuite) TestRecoverAllIgnoresTruncatedManifestTail() {
	buf := s.newBuffer()

	id := disk.NewPageID("table/main.db", 9)
	record := encodeCopyOnWriteRecordForTest(id, 1, 4, []byte("stable"))
	truncatedTail := encodeCopyOnWriteRecordForTest(disk.NewPageID("table/main.db", 10), 2, 7, []byte("tail"))[:9]
	manifest := append(record, truncatedTail...)

	s.expectManifestReads(manifest)
	s.Require().NoError(buf.RecoverAll())

	shadowID, ok := buf.ResolvePage(id)
	s.True(ok)
	s.Equal(disk.NewPageID("copyonwrite.pages", 4), shadowID)
}

func (s *CopyOnWriteBufferTestSuite) newBuffer() *Buffer {
	s.store.EXPECT().FileSize("copyonwrite.manifest").Return(int64(0), nil)

	buf, err := NewBuffer(s.store)
	s.Require().NoError(err)
	return buf
}

func (s *CopyOnWriteBufferTestSuite) expectManifestReads(manifest []byte) {
	s.store.EXPECT().FileSize("copyonwrite.manifest").Return(int64(len(manifest)), nil)
	s.store.EXPECT().
		ReadFileAt("copyonwrite.manifest", gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ string, offset int64, buf []byte) (int, error) {
			if offset >= int64(len(manifest)) {
				return 0, io.EOF
			}

			n := copy(buf, manifest[offset:])
			if n < len(buf) {
				return n, io.EOF
			}

			return n, nil
		})
}

type copyOnWriteRecordForTest struct {
	pageID        disk.PageID
	sequence      uint64
	shadowPageNum int64
	checksum      uint32
}

func encodeCopyOnWriteRecordForTest(id disk.PageID, sequence uint64, shadowPageNum int64, payload []byte) []byte {
	const (
		recordTypePageVersion = 1
		lengthFieldSize       = 4
		checksumFieldSize     = 4
		recordTypeSize        = 1
		sequenceSize          = 8
		shadowPageNumSize     = 8
		pageChecksumSize      = 4
		fileNameLenSize       = 2
		logicalPageNumSize    = 8
		recordPrefixSize      = checksumFieldSize + recordTypeSize + sequenceSize + shadowPageNumSize + pageChecksumSize + fileNameLenSize + logicalPageNumSize
		headerSize            = lengthFieldSize + checksumFieldSize
	)

	fileName := id.FileName()
	page := disk.NewPage(id)
	_ = page.Write(payload)
	bodyLen := recordPrefixSize + len(fileName)
	buf := bytes.NewBuffer(make([]byte, 0, lengthFieldSize+bodyLen))
	_ = binary.Write(buf, config.ByteOrder, uint32(bodyLen))
	_ = binary.Write(buf, config.ByteOrder, uint32(0))
	_ = buf.WriteByte(recordTypePageVersion)
	_ = binary.Write(buf, config.ByteOrder, sequence)
	_ = binary.Write(buf, config.ByteOrder, uint64(shadowPageNum))
	_ = binary.Write(buf, config.ByteOrder, disk.Checksum(page.Data()))
	_ = binary.Write(buf, config.ByteOrder, uint16(len(fileName)))
	_, _ = buf.WriteString(fileName)
	_ = binary.Write(buf, config.ByteOrder, uint64(id.PageNum()))

	encoded := buf.Bytes()
	config.ByteOrder.PutUint32(encoded[lengthFieldSize:headerSize], disk.Checksum(encoded[headerSize:]))
	return encoded
}

func decodeCopyOnWriteRecordForTest(encoded []byte) copyOnWriteRecordForTest {
	const (
		lengthFieldSize   = 4
		checksumFieldSize = 4
	)

	body := encoded[lengthFieldSize:]
	reader := bytes.NewReader(body[checksumFieldSize:])
	_, _ = reader.ReadByte()

	record := copyOnWriteRecordForTest{}
	_ = binary.Read(reader, config.ByteOrder, &record.sequence)
	var shadowPageNum uint64
	_ = binary.Read(reader, config.ByteOrder, &shadowPageNum)
	record.shadowPageNum = int64(shadowPageNum)
	_ = binary.Read(reader, config.ByteOrder, &record.checksum)
	var fileNameLen uint16
	_ = binary.Read(reader, config.ByteOrder, &fileNameLen)
	fileName := make([]byte, fileNameLen)
	_, _ = io.ReadFull(reader, fileName)
	var logicalPageNum uint64
	_ = binary.Read(reader, config.ByteOrder, &logicalPageNum)
	record.pageID = disk.NewPageID(string(fileName), int64(logicalPageNum))
	return record
}
