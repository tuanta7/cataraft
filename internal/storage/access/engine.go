package storage

type Engine interface {
	Set(key, value []byte)
	Get(key []byte) []byte
}
