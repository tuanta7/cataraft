package execution

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type ManagerTestSuite struct {
	suite.Suite
}

func TestManagerTestSuite(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}

func (s *ManagerTestSuite) TestFlushPageFlushesWALBeforeBuffer() {
	id := disk.NewPageID("table.db", 1)
	page := disk.NewPage(id)
	page.SetLSN(9)

	buf := &stubBuffer{page: page}
	wal := &stubWAL{flushedLSN: 1}
	mgr := NewManager(nil, nil, buf, wal)

	err := mgr.FlushPage(id)

	s.Require().NoError(err)
	s.Equal([]string{"read", "flush"}, buf.calls)
	s.Equal([]uint64{9}, wal.flushThroughCalls)
}

func (s *ManagerTestSuite) TestFlushPageSkipsWALIfAlreadyFlushed() {
	id := disk.NewPageID("table.db", 2)
	page := disk.NewPage(id)
	page.SetLSN(3)

	buf := &stubBuffer{page: page}
	wal := &stubWAL{flushedLSN: 3}
	mgr := NewManager(nil, nil, buf, wal)

	err := mgr.FlushPage(id)

	s.Require().NoError(err)
	s.Equal([]string{"read", "flush"}, buf.calls)
	s.Empty(wal.flushThroughCalls)
}

func (s *ManagerTestSuite) TestFlushPageReturnsWALFlushError() {
	id := disk.NewPageID("table.db", 3)
	page := disk.NewPage(id)
	page.SetLSN(5)

	buf := &stubBuffer{page: page}
	wal := &stubWAL{flushedLSN: 0, flushErr: errors.New("logger fsync failed")}
	mgr := NewManager(nil, nil, buf, wal)

	err := mgr.FlushPage(id)

	s.ErrorContains(err, "logger fsync failed")
	s.Equal([]string{"read"}, buf.calls)
}

type stubBuffer struct {
	page     *disk.Page
	readErr  error
	flushErr error
	calls    []string
}

func (b *stubBuffer) ReadPage(_ disk.PageID) (*disk.Page, error) {
	b.calls = append(b.calls, "read")
	if b.readErr != nil {
		return nil, b.readErr
	}

	return b.page, nil
}

func (b *stubBuffer) Flush(_ disk.PageID) error {
	b.calls = append(b.calls, "flush")
	return b.flushErr
}

type stubWAL struct {
	flushedLSN        uint64
	flushErr          error
	flushThroughCalls []uint64
}

func (w *stubWAL) FlushedLSN() uint64 {
	return w.flushedLSN
}

func (w *stubWAL) FlushThrough(lsn uint64) error {
	w.flushThroughCalls = append(w.flushThroughCalls, lsn)
	if w.flushErr != nil {
		return w.flushErr
	}
	w.flushedLSN = lsn
	return nil
}
