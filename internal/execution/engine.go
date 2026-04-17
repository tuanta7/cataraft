package execution

import (
	"errors"

	"github.com/tuanta7/cataraft/internal/query"
)

type Index interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
}

type Flusher interface {
	FlushAll() error
}

type Engine struct {
	parser  *query.Parser
	index   Index // single table
	flusher Flusher
}

func NewEngine(parser *query.Parser, index Index, flusher Flusher) (*Engine, error) {
	if parser == nil {
		return nil, errors.New("parser is required")
	}
	if index == nil {
		return nil, errors.New("index is required")
	}
	if flusher == nil {
		return nil, errors.New("flusher is required")
	}

	return &Engine{
		parser:  parser,
		index:   index,
		flusher: flusher,
	}, nil
}

func (e *Engine) Exec(input string) ([]byte, error) {
	cmd, err := e.parser.Parse(input)
	if err != nil {
		return nil, err
	}

	switch cmd.Type {
	case query.CommandGet:
		return e.index.Get([]byte(cmd.Key))
	case query.CommandSet:
		return nil, e.index.Put([]byte(cmd.Key), []byte(cmd.Value))
	default:
		return nil, query.ErrUnsupportedCommand
	}
}

func (e *Engine) Flush() error {
	return e.flusher.FlushAll()
}
