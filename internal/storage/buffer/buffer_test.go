package buffer_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	buffer "github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/mocks"
	gomock "go.uber.org/mock/gomock"
)

type CoreBufferTestSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *mocks.MockPageStore
	writer *mocks.MockDirtyPageWriter
}

func TestCoreBufferTestSuite(t *testing.T) {
	suite.Run(t, new(CoreBufferTestSuite))
}

func (s *CoreBufferTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = mocks.NewMockPageStore(s.ctrl)
	s.writer = mocks.NewMockDirtyPageWriter(s.ctrl)
}

func (s *CoreBufferTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CoreBufferTestSuite) TestEvictionFlushesDirtyVictim() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	buf := buffer.NewLRUBuffer(1, s.store, nil)
	firstPage := disk.NewPage(firstID)
	secondPage := disk.NewPage(secondID)
	var flushed *disk.Page

	gomock.InOrder(
		s.store.EXPECT().ReadPage(firstID).Return(firstPage, nil),
		s.store.EXPECT().ReadPage(secondID).Return(secondPage, nil),
		s.store.EXPECT().WritePage(firstID, gomock.Any()).DoAndReturn(func(id disk.PageID, page *disk.Page) error {
			flushed = page
			return nil
		}),
	)

	s.Require().NoError(buf.WritePage(firstID, []byte("first")))
	_, err := buf.ReadPage(secondID)
	s.Require().NoError(err)

	s.Require().NotNil(flushed)
	s.Equal([]byte("first"), flushed.Data()[:5])
	s.False(buf.Contains(firstID))
	s.True(buf.Contains(secondID))
}

func (s *CoreBufferTestSuite) TestPinnedPageIsNotEvicted() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	thirdID := disk.NewPageID("table.db", 3)
	buf := buffer.NewLRUBuffer(2, s.store, nil)

	gomock.InOrder(
		s.store.EXPECT().ReadPage(firstID).Return(disk.NewPage(firstID), nil),
		s.store.EXPECT().ReadPage(secondID).Return(disk.NewPage(secondID), nil),
		s.store.EXPECT().ReadPage(thirdID).Return(disk.NewPage(thirdID), nil),
	)

	_, err := buf.ReadPage(firstID)
	s.Require().NoError(err)
	_, err = buf.ReadPage(secondID)
	s.Require().NoError(err)
	s.Require().NoError(buf.Pin(firstID))

	_, err = buf.ReadPage(thirdID)
	s.Require().NoError(err)

	s.True(buf.Contains(firstID))
	s.False(buf.Contains(secondID))
	s.True(buf.Contains(thirdID))
}

func (s *CoreBufferTestSuite) TestUnpinAllowsEviction() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	thirdID := disk.NewPageID("table.db", 3)
	buf := buffer.NewLRUBuffer(2, s.store, nil)

	gomock.InOrder(
		s.store.EXPECT().ReadPage(firstID).Return(disk.NewPage(firstID), nil),
		s.store.EXPECT().ReadPage(secondID).Return(disk.NewPage(secondID), nil),
		s.store.EXPECT().ReadPage(thirdID).Return(disk.NewPage(thirdID), nil),
	)

	_, err := buf.ReadPage(firstID)
	s.Require().NoError(err)
	_, err = buf.ReadPage(secondID)
	s.Require().NoError(err)
	s.Require().NoError(buf.Pin(firstID))
	s.Require().NoError(buf.Unpin(firstID))

	_, err = buf.ReadPage(thirdID)
	s.Require().NoError(err)

	s.False(buf.Contains(firstID))
	s.True(buf.Contains(secondID))
	s.True(buf.Contains(thirdID))
}
