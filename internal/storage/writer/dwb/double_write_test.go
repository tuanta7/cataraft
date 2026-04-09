package dwb_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	recovery "github.com/tuanta7/cataraft/internal/storage/writer/dwb"
	"github.com/tuanta7/cataraft/mocks"
	gomock "go.uber.org/mock/gomock"
)

type DoubleWriteBufferTestSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *mocks.MockDoubleWriteStore
	buf   *recovery.DoubleWriteBuffer
}

func TestDoubleWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(DoubleWriteBufferTestSuite))
}

func (s *DoubleWriteBufferTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = mocks.NewMockDoubleWriteStore(s.ctrl)

	buf, err := recovery.NewDoubleWriteBuffer(s.store, "doublewrite.db")
	s.Require().NoError(err)
	s.buf = buf
}

func (s *DoubleWriteBufferTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *DoubleWriteBufferTestSuite) TestStagePageKeepsLocalCopy() {
	id := disk.NewPageID("table/main.db", 1)
	page := disk.NewPage(id)
	s.Require().NoError(page.Write([]byte("stage")))

	s.Require().NoError(s.buf.StagePage(id, page))
	s.True(s.buf.HasPage(id))
}

func (s *DoubleWriteBufferTestSuite) TestFlushPageWritesMetadataDataThenPrimaryAndClearsSlot() {
	id := disk.NewPageID("table/main.db", 2)
	page := disk.NewPage(id)
	page.SetLSN(9)
	s.Require().NoError(page.Write([]byte("flush")))
	s.Require().NoError(s.buf.StagePage(id, page))

	metaID := disk.NewPageID("doublewrite.db", 0)
	dataID := disk.NewPageID("doublewrite.db", 1)

	gomock.InOrder(
		s.store.EXPECT().WritePage(metaID, gomock.Any()).Return(nil),
		s.store.EXPECT().WritePage(dataID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile("doublewrite.db").Return(nil),
		s.store.EXPECT().WritePage(id, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile(id.FileName()).Return(nil),
		s.store.EXPECT().WritePage(metaID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile("doublewrite.db").Return(nil),
	)

	s.Require().NoError(s.buf.FlushPage(id))
	s.False(s.buf.HasPage(id))
}

func (s *DoubleWriteBufferTestSuite) TestRecoverAllRestoresLatestActiveEntryPerPage() {
	targetID := disk.NewPageID("table/main.db", 7)
	metaOldID := disk.NewPageID("doublewrite.db", 0)
	dataOldID := disk.NewPageID("doublewrite.db", 1)
	metaNewID := disk.NewPageID("doublewrite.db", 2)
	dataNewID := disk.NewPageID("doublewrite.db", 3)

	oldData := disk.NewPage(dataOldID)
	s.Require().NoError(oldData.Write([]byte("old")))
	newData := disk.NewPage(dataNewID)
	s.Require().NoError(newData.Write([]byte("newer")))

	oldMeta := s.mustMetadataPage(metaOldID, true, 1, targetID, 3, oldData.Data())
	newMeta := s.mustMetadataPage(metaNewID, true, 2, targetID, 4, newData.Data())
	emptyMeta := disk.NewPage(disk.NewPageID("doublewrite.db", 4))

	s.store.EXPECT().
		ReadPage(gomock.Any()).
		AnyTimes().
		DoAndReturn(func(id disk.PageID) (*disk.Page, error) {
			switch id {
			case metaOldID:
				return oldMeta, nil
			case dataOldID:
				return oldData, nil
			case metaNewID:
				return newMeta, nil
			case dataNewID:
				return newData, nil
			default:
				return emptyMeta, nil
			}
		})

	gomock.InOrder(
		s.store.EXPECT().WritePage(targetID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile(targetID.FileName()).Return(nil),
		s.store.EXPECT().WritePage(metaOldID, gomock.Any()).Return(nil),
		s.store.EXPECT().WritePage(metaNewID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile("doublewrite.db").Return(nil),
	)

	s.Require().NoError(s.buf.RecoverAll())
}

func (s *DoubleWriteBufferTestSuite) TestRecoverPageFindsEntryByMetadata() {
	targetID := disk.NewPageID("table/main.db", 3)
	metaID := disk.NewPageID("doublewrite.db", 0)
	dataID := disk.NewPageID("doublewrite.db", 1)
	dataPage := disk.NewPage(dataID)
	s.Require().NoError(dataPage.Write([]byte("recover")))
	metaPage := s.mustMetadataPage(metaID, true, 1, targetID, 11, dataPage.Data())
	emptyMeta := disk.NewPage(disk.NewPageID("doublewrite.db", 4))

	s.store.EXPECT().
		ReadPage(gomock.Any()).
		AnyTimes().
		DoAndReturn(func(id disk.PageID) (*disk.Page, error) {
			switch id {
			case metaID:
				return metaPage, nil
			case dataID:
				return dataPage, nil
			default:
				return emptyMeta, nil
			}
		})

	gomock.InOrder(
		s.store.EXPECT().WritePage(targetID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile(targetID.FileName()).Return(nil),
		s.store.EXPECT().WritePage(metaID, gomock.Any()).Return(nil),
		s.store.EXPECT().SyncFile("doublewrite.db").Return(nil),
	)

	s.Require().NoError(s.buf.RecoverPage(targetID))
}

func (s *DoubleWriteBufferTestSuite) mustMetadataPage(id disk.PageID, active bool, sequence uint64, targetID disk.PageID, lsn uint64, payload []byte) *disk.Page {
	s.T().Helper()

	raw := encodeDoubleWriteMetadataForTest(active, sequence, targetID, lsn, payload)
	page := disk.NewPage(id)
	s.Require().NoError(page.Reset(raw))
	return page
}

func encodeDoubleWriteMetadataForTest(active bool, sequence uint64, targetID disk.PageID, lsn uint64, payload []byte) []byte {
	const (
		doubleWriteMagic = "CATADWB1"
		activeFlag       = 1
		magicSize        = 8
		activeSize       = 1
		reservedSize     = 7
		sequenceSize     = 8
		pageNumSize      = 8
		lsnSize          = 8
		checksumSize     = 4
		fileNameLenSize  = 2
		metadataBaseSize = magicSize + activeSize + reservedSize + sequenceSize + pageNumSize + lsnSize + checksumSize + fileNameLenSize
	)

	fileName := targetID.FileName()
	buf := bytes.NewBuffer(make([]byte, 0, metadataBaseSize+len(fileName)))
	_, _ = buf.WriteString(doubleWriteMagic)
	flag := byte(0)
	if active {
		flag = activeFlag
	}
	_ = buf.WriteByte(flag)
	_, _ = buf.Write(make([]byte, reservedSize))
	_ = binary.Write(buf, config.ByteOrder, sequence)
	_ = binary.Write(buf, config.ByteOrder, uint64(targetID.PageNum()))
	_ = binary.Write(buf, config.ByteOrder, lsn)
	_ = binary.Write(buf, config.ByteOrder, checksumForTest(payload))
	_ = binary.Write(buf, config.ByteOrder, uint16(len(fileName)))
	_, _ = buf.WriteString(fileName)
	return buf.Bytes()
}

func checksumForTest(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
