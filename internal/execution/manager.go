package execution

import "github.com/tuanta7/cataraft/internal/storage/disk"

type Parser interface{}

type Indexer interface {
}

type Buffer interface {
	// ReadPage reads a page from the buffer.
	ReadPage(id disk.PageID) (*disk.Page, error)
	Flush(id disk.PageID) error
}

type Logger interface {
	// FlushedLSN returns the LSN of the last flushed Logger entry.
	FlushedLSN() uint64
	// FlushThrough flushes all Logger entries up to and including the given LSN.
	FlushThrough(lsn uint64) error
}

type Manager struct {
	parser  Parser
	indexer Indexer
	buffer  Buffer
	logger  Logger
}

func NewManager(parser Parser, indexer Indexer, buffer Buffer, wal Logger) *Manager {
	return &Manager{
		parser:  parser,
		indexer: indexer,
		buffer:  buffer,
		logger:  wal,
	}
}

// getTable reads the root table file from the disk and returns a Table object.
func (m *Manager) getTable(tableName string) (Table, error) {
	return Table{}, nil
}
