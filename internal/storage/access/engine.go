package access

type Engine interface {
	Get(key []byte) ([]byte, error)
	Insert(key, value []byte)
	Delete(key []byte)
	Encode() []byte
}
