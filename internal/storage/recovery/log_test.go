package recovery_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	recovery "github.com/tuanta7/cataraft/internal/storage/recovery"
	"github.com/tuanta7/cataraft/mocks"
	gomock "go.uber.org/mock/gomock"
)

type WriteAheadLogTestSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *mocks.MockLogStore
	wal   *recovery.WriteAheadLog
}

func TestWriteAheadLogTestSuite(t *testing.T) {
	suite.Run(t, new(WriteAheadLogTestSuite))
}

func (s *WriteAheadLogTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = mocks.NewMockLogStore(s.ctrl)

	s.store.EXPECT().FileSize("wal.log").Return(int64(0), nil)

	wal, err := recovery.NewWriteAheadLog(s.store, "wal.log")
	s.Require().NoError(err)
	s.wal = wal
}

func (s *WriteAheadLogTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *WriteAheadLogTestSuite) TestAppendPageWriteMutatesPageAndPersistsRecord() {
	page := disk.NewPage(disk.NewPageID("table/main.db", 1))
	var appended []byte

	s.store.EXPECT().
		AppendFile("wal.log", gomock.Any()).
		DoAndReturn(func(_ string, data []byte) (int64, error) {
			appended = bytes.Clone(data)
			return 0, nil
		})
	s.store.EXPECT().
		FileSize("wal.log").
		DoAndReturn(func(string) (int64, error) {
			return int64(len(appended)), nil
		})
	s.expectReadRecords("wal.log", func() []byte { return appended })

	record, err := s.wal.AppendPageWrite(page, 0, []byte("cataraft"))
	s.Require().NoError(err)

	s.Equal(uint64(1), record.LSN)
	s.Equal(uint64(1), page.LSN())
	s.Equal([]byte("cataraft"), page.Data()[:8])
	s.Equal(uint64(1), s.wal.LastLSN())

	records, err := s.wal.Records()
	s.Require().NoError(err)
	s.Require().Len(records, 1)
	s.Equal(record.LSN, records[0].LSN)
	s.Equal(record.PageID, records[0].PageID)
	s.Equal(record.Payload, records[0].Payload)
}

func (s *WriteAheadLogTestSuite) TestFlushTracksFlushedLSN() {
	page := disk.NewPage(disk.NewPageID("table/main.db", 2))

	s.store.EXPECT().AppendFile("wal.log", gomock.Any()).Return(int64(0), nil)
	s.store.EXPECT().SyncFile("wal.log").Return(nil)

	_, err := s.wal.AppendPageWrite(page, 4, []byte("raft"))
	s.Require().NoError(err)
	s.Equal(uint64(0), s.wal.FlushedLSN())

	s.Require().NoError(s.wal.Flush())
	s.Equal(uint64(1), s.wal.FlushedLSN())
}

func (s *WriteAheadLogTestSuite) TestNewWriteAheadLogReloadsExistingRecords() {
	recordOne, err := encodeRecordForTest(recovery.LogRecord{
		LSN:     1,
		PageID:  disk.NewPageID("table/main.db", 3),
		Offset:  0,
		Payload: []byte("one"),
	})
	s.Require().NoError(err)
	recordTwo, err := encodeRecordForTest(recovery.LogRecord{
		LSN:     2,
		PageID:  disk.NewPageID("table/main.db", 3),
		Offset:  3,
		Payload: []byte("two"),
	})
	s.Require().NoError(err)
	walBytes := append(recordOne, recordTwo...)

	s.store.EXPECT().FileSize("reload.log").Return(int64(len(walBytes)), nil)
	s.expectReadRecords("reload.log", func() []byte { return walBytes })

	reloaded, err := recovery.NewWriteAheadLog(s.store, "reload.log")
	s.Require().NoError(err)
	s.Equal(uint64(2), reloaded.LastLSN())
	s.Equal(uint64(2), reloaded.FlushedLSN())
}

func (s *WriteAheadLogTestSuite) TestReplayAppliesRecordsInOrder() {
	recordOne, err := encodeRecordForTest(recovery.LogRecord{
		LSN:     1,
		PageID:  disk.NewPageID("table/main.db", 4),
		Offset:  0,
		Payload: []byte("hello"),
	})
	s.Require().NoError(err)
	recordTwo, err := encodeRecordForTest(recovery.LogRecord{
		LSN:     2,
		PageID:  disk.NewPageID("table/main.db", 4),
		Offset:  5,
		Payload: []byte("-raft"),
	})
	s.Require().NoError(err)
	walBytes := append(recordOne, recordTwo...)

	s.store.EXPECT().FileSize("wal.log").Return(int64(len(walBytes)), nil)
	s.expectReadRecords("wal.log", func() []byte { return walBytes })

	replayPage := disk.NewPage(disk.NewPageID("table/main.db", 4))
	err = s.wal.Replay(func(record recovery.LogRecord) error {
		return recovery.Apply(record, replayPage)
	})
	s.Require().NoError(err)

	s.Equal([]byte("hello-raft"), replayPage.Data()[:10])
	s.Equal(uint64(2), replayPage.LSN())
}

func (s *WriteAheadLogTestSuite) expectReadRecords(fileName string, dataFn func() []byte) {
	s.store.EXPECT().
		ReadFileAt(fileName, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ string, offset int64, buf []byte) (int, error) {
			data := dataFn()
			end := offset + int64(len(buf))
			copy(buf, data[offset:end])
			return len(buf), nil
		}).
		AnyTimes()
}

func encodeRecordForTest(record recovery.LogRecord) ([]byte, error) {
	const (
		recordTypePageWrite uint8 = 1
		lengthFieldSize           = 4
		checksumFieldSize         = 4
		recordTypeSize            = 1
		lsnFieldSize              = 8
		fileNameLenSize           = 2
		pageNumFieldSize          = 8
		offsetFieldSize           = 4
		payloadLenSize            = 4
	)

	fileName := record.PageID.FileName()
	bodyLen := checksumFieldSize + recordTypeSize + lsnFieldSize + fileNameLenSize + len(fileName) + pageNumFieldSize + offsetFieldSize + payloadLenSize + len(record.Payload)
	buf := bytes.NewBuffer(make([]byte, 0, lengthFieldSize+bodyLen))

	if err := binary.Write(buf, binary.BigEndian, uint32(bodyLen)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(0)); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(recordTypePageWrite); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, record.LSN); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(len(fileName))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(fileName); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint64(record.PageID.PageNum())); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, record.Offset); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(record.Payload))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(record.Payload); err != nil {
		return nil, err
	}

	encoded := buf.Bytes()
	binary.BigEndian.PutUint32(encoded[lengthFieldSize:lengthFieldSize+checksumFieldSize], crc32.ChecksumIEEE(encoded[lengthFieldSize+checksumFieldSize:]))
	return encoded, nil
}
