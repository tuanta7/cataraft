package disk

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"
)

type AdapterTestSuite struct {
	suite.Suite
	adapter *Adapter
}

func TestAdapterTestSuite(t *testing.T) {
	suite.Run(t, new(AdapterTestSuite))
}

func (s *AdapterTestSuite) SetupTest() {
	adapter, err := NewAdapter(AdapterConfig{
		BaseDir: s.T().TempDir(),
		Direct:  false,
	})
	s.Require().NoError(err)
	s.adapter = adapter
}

func (s *AdapterTestSuite) TearDownTest() {
	s.Require().NoError(s.adapter.Close())
}

func (s *AdapterTestSuite) TestWriteAndReadPage() {
	id := NewPageID("table/main.db", 2)
	page := NewPage(id)
	payload := []byte("cataraft")
	s.Require().NoError(page.Write(payload))

	s.Require().NoError(s.adapter.WritePage(id, page))
	s.Require().NoError(s.adapter.SyncFile(id.FileName()))

	got, err := s.adapter.ReadPage(id)
	s.Require().NoError(err)

	s.True(bytes.Equal(got.Data()[:len(payload)], payload))
	s.False(got.IsDirty())
}

func (s *AdapterTestSuite) TestReadMissingPageReturnsZeroedPage() {
	got, err := s.adapter.ReadPage(NewPageID("empty.db", 10))
	s.Require().NoError(err)

	s.Len(bytes.Trim(got.Data(), "\x00"), 0)
}

func (s *AdapterTestSuite) TestRejectsPathTraversal() {
	_, err := s.adapter.ReadPage(NewPageID("../escape.db", 0))
	s.Require().Error(err)
}
