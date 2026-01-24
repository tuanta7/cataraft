package storage

type Engine interface {
	Get(key []byte) ([]byte, error)
	Put(key []byte, value []byte) error
	Flush() error
}
