package disk

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PageTestSuite struct {
	suite.Suite
}

func TestPageTestSuite(t *testing.T) {
	suite.Run(t, new(PageTestSuite))
}

func (s *PageTestSuite) TestDataReturnsCopy() {
	page := NewPage(NewPageID("main.db", 0))
	s.Require().NoError(page.Write([]byte("cataraft")))

	data := page.Data()
	data[0] = 'X'

	s.Equal(byte('c'), page.Data()[0])
	s.True(page.IsDirty())
}

func (s *PageTestSuite) TestWriteAt() {
	page := NewPage(NewPageID("main.db", 0))
	s.Require().NoError(page.Reset([]byte("cataraft")))
	s.Require().NoError(page.WriteAt(2, []byte("XYZ")))

	s.Equal([]byte("caXYZaft"), page.Data()[:8])
	s.True(page.IsDirty())
}

func (s *PageTestSuite) TestMarkClean() {
	page := NewPage(NewPageID("main.db", 0))
	s.Require().NoError(page.Write([]byte("cataraft")))

	page.MarkClean()

	s.False(page.IsDirty())
}

func (s *PageTestSuite) TestPageIDValidate() {
	s.NoError(NewPageID("main.db", 0).Validate())
	s.Error(NewPageID("", 0).Validate())
	s.Error(NewPageID("main.db", -1).Validate())
}
