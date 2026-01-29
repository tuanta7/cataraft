package strategy

type Eviction[T comparable] interface {
	OnAccess(id T)
	OnEvict() (T, error)
}
