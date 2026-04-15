package execution

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tuanta7/cataraft/internal/query"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/buffer/copyonwrite"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/index/bptree"
)

type EngineIntegrationTestSuite struct {
	suite.Suite
	baseDir string
	adapter *disk.Adapter
	engine  *Engine
}

func TestEngineIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(EngineIntegrationTestSuite))
}

func (s *EngineIntegrationTestSuite) SetupTest() {
	s.baseDir = s.T().TempDir()
	s.openEngine()
}

func (s *EngineIntegrationTestSuite) TearDownTest() {
	if s.adapter != nil {
		s.Require().NoError(s.adapter.Close())
	}
}

func (s *EngineIntegrationTestSuite) TestSetGetAndReload() {
	result, err := s.engine.Exec("SET greeting hello")
	s.Require().NoError(err)
	s.Nil(result)
	s.Require().NoError(s.engine.Flush())

	result, err = s.engine.Exec("GET greeting")
	s.Require().NoError(err)
	s.Equal([]byte("hello"), result)

	s.Require().NoError(s.adapter.Close())
	s.adapter = nil

	s.openEngine()

	result, err = s.engine.Exec("GET greeting")
	s.Require().NoError(err)
	s.Equal([]byte("hello"), result)
}

func (s *EngineIntegrationTestSuite) TestUnsupportedQueryFails() {
	_, err := s.engine.Exec("DELETE greeting")
	s.Require().Error(err)
}

func (s *EngineIntegrationTestSuite) openEngine() {
	var err error
	s.adapter, err = disk.NewAdapter(s.baseDir)
	s.Require().NoError(err)

	copyOnWrite, err := copyonwrite.NewBuffer(s.adapter)
	s.Require().NoError(err)

	lru := buffer.NewLRUBuffer(128, copyOnWrite)
	tree, err := bptree.New(bptree.DefaultOrder, lru)
	s.Require().NoError(err)

	s.engine, err = NewEngine(query.NewParser(), tree, lru)
	s.Require().NoError(err)
}
