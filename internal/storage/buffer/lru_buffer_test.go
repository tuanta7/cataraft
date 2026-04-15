package buffer

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type LRUBufferTestSuite struct {
	suite.Suite
	store *disk.Adapter
}

func TestLRUBufferTestSuite(t *testing.T) {
	suite.Run(t, new(LRUBufferTestSuite))
}

func (s *LRUBufferTestSuite) SetupTest() {
	var err error
	s.store, err = disk.NewAdapter(s.T().TempDir())
	s.Require().NoError(err)
}

func (s *LRUBufferTestSuite) TearDownTest() {
	s.Require().NoError(s.store.Close())
}

func (s *LRUBufferTestSuite) TestEvictionFlushesDirtyVictim() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	buf := NewLRUBuffer(1, s.store)

	s.Require().NoError(buf.WritePage(firstID, []byte("first")))
	_, err := buf.ReadPage(secondID)
	s.Require().NoError(err)

	flushed, err := s.store.ReadPage(firstID)
	s.Require().NoError(err)
	s.Equal([]byte("first"), flushed.Data()[:5])
	s.False(buf.Contains(firstID))
	s.True(buf.Contains(secondID))
}

func (s *LRUBufferTestSuite) TestPinnedPageIsNotEvicted() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	thirdID := disk.NewPageID("table.db", 3)
	buf := NewLRUBuffer(2, s.store)

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

func (s *LRUBufferTestSuite) TestUnpinAllowsEviction() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	thirdID := disk.NewPageID("table.db", 3)
	buf := NewLRUBuffer(2, s.store)

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

func (s *LRUBufferTestSuite) TestTouchMakesPageMostRecentlyUsed() {
	firstID := disk.NewPageID("table.db", 1)
	secondID := disk.NewPageID("table.db", 2)
	thirdID := disk.NewPageID("table.db", 3)
	buf := NewLRUBuffer(2, s.store)

	_, err := buf.ReadPage(firstID)
	s.Require().NoError(err)
	_, err = buf.ReadPage(secondID)
	s.Require().NoError(err)

	// Re-read the first page so the second page becomes the eviction victim.
	_, err = buf.ReadPage(firstID)
	s.Require().NoError(err)

	_, err = buf.ReadPage(thirdID)
	s.Require().NoError(err)

	s.True(buf.Contains(firstID))
	s.False(buf.Contains(secondID))
	s.True(buf.Contains(thirdID))
}
