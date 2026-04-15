package bptree

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/buffer/copyonwrite"
	"github.com/tuanta7/cataraft/internal/storage/disk"
)

type BPlusTreeTestSuite struct {
	suite.Suite
	tree    *BPlusTree
	baseDir string
	adapter *disk.Adapter
	buf     *buffer.LRUBuffer
	cow     *copyonwrite.Buffer
}

func TestBPlusTreeTestSuite(t *testing.T) {
	suite.Run(t, new(BPlusTreeTestSuite))
}

func (s *BPlusTreeTestSuite) SetupTest() {
	s.baseDir = s.T().TempDir()
	var err error

	s.adapter, err = disk.NewAdapter(s.baseDir)
	s.Require().NoError(err)
	s.cow, err = copyonwrite.NewBuffer(s.adapter)
	s.Require().NoError(err)
	s.buf = buffer.NewLRUBuffer(128, s.cow)

	tree, err := New(4, s.buf)
	s.Require().NoError(err)
	s.tree = tree
}

func (s *BPlusTreeTestSuite) TearDownTest() {
	s.Require().NoError(s.adapter.Close())
}

func (s *BPlusTreeTestSuite) TestPutGetAndUpdate() {
	s.Require().NoError(s.tree.Put([]byte("alpha"), []byte("1")))
	s.Require().NoError(s.tree.Put([]byte("bravo"), []byte("2")))
	s.Require().NoError(s.tree.Put([]byte("charlie"), []byte("3")))
	s.Require().NoError(s.tree.Put([]byte("bravo"), []byte("updated")))

	value, err := s.tree.Get([]byte("bravo"))
	s.Require().NoError(err)
	s.Equal([]byte("updated"), value)
	s.Equal(3, s.tree.Len())
}

func (s *BPlusTreeTestSuite) TestInsertSplitAndScan() {
	for i := 1; i <= 10; i++ {
		key := []byte(fmt.Sprintf("%02d", i))
		value := []byte(fmt.Sprintf("v%02d", i))
		s.Require().NoError(s.tree.Put(key, value))
	}

	entries := s.tree.Scan(nil, nil)
	s.Len(entries, 10)
	for i, entry := range entries {
		expectedKey := fmt.Sprintf("%02d", i+1)
		expectedValue := fmt.Sprintf("v%02d", i+1)
		s.Equal([]byte(expectedKey), entry.Key)
		s.Equal([]byte(expectedValue), entry.Value)
	}
	s.GreaterOrEqual(s.tree.Height(), 2)
	s.NotEmpty(s.tree.RootKeys())
}

func (s *BPlusTreeTestSuite) TestScanWithBounds() {
	for i := 1; i <= 8; i++ {
		key := []byte(fmt.Sprintf("%02d", i))
		value := []byte(fmt.Sprintf("v%02d", i))
		s.Require().NoError(s.tree.Put(key, value))
	}

	entries := s.tree.Scan([]byte("03"), []byte("07"))
	s.Len(entries, 4)
	s.Equal([]byte("03"), entries[0].Key)
	s.Equal([]byte("06"), entries[len(entries)-1].Key)
}

func (s *BPlusTreeTestSuite) TestDeleteRebalancesAndShrinksRoot() {
	for i := 1; i <= 8; i++ {
		key := []byte(fmt.Sprintf("%02d", i))
		value := []byte(fmt.Sprintf("v%02d", i))
		s.Require().NoError(s.tree.Put(key, value))
	}

	for i := 1; i <= 7; i++ {
		s.Require().NoError(s.tree.Delete([]byte(fmt.Sprintf("%02d", i))))
	}

	entries := s.tree.Scan(nil, nil)
	s.Len(entries, 1)
	s.Equal([]byte("08"), entries[0].Key)
	s.Equal(1, s.tree.Height())

	value, err := s.tree.Get([]byte("08"))
	s.Require().NoError(err)
	s.Equal([]byte("v08"), value)
}

func (s *BPlusTreeTestSuite) TestDeleteMissingKeyReturnsError() {
	s.Require().NoError(s.tree.Put([]byte("alpha"), []byte("1")))

	err := s.tree.Delete([]byte("missing"))

	s.ErrorIs(err, ErrKeyNotFound)
}

func (s *BPlusTreeTestSuite) TestReloadFromDiskViaBuffer() {
	for i := 1; i <= 6; i++ {
		key := []byte(fmt.Sprintf("%02d", i))
		value := []byte(fmt.Sprintf("v%02d", i))
		s.Require().NoError(s.tree.Put(key, value))
	}
	s.Require().NoError(s.tree.Delete([]byte("03")))
	s.Require().NoError(s.adapter.Close())

	reopenAdapter, err := disk.NewAdapter(s.baseDir)
	s.Require().NoError(err)
	s.adapter = reopenAdapter
	s.cow, err = copyonwrite.NewBuffer(s.adapter)
	s.Require().NoError(err)
	s.buf = buffer.NewLRUBuffer(128, s.cow)

	reloaded, err := New(4, s.buf)
	s.Require().NoError(err)

	entries := reloaded.Scan(nil, nil)
	s.Len(entries, 5)
	s.Equal([]byte("01"), entries[0].Key)
	s.Equal([]byte("02"), entries[1].Key)
	s.Equal([]byte("04"), entries[2].Key)
	s.Equal([]byte("05"), entries[3].Key)
	s.Equal([]byte("06"), entries[4].Key)

	value, getErr := reloaded.Get([]byte("05"))
	s.Require().NoError(getErr)
	s.Equal([]byte("v05"), value)
}
