package recovery

type Strategy interface {
	Write(path string, data []byte)
}

type CopyOnWrite struct{}

func (c *CopyOnWrite) Write(path string, data []byte) {}

type DoubleWrite struct{}

func (d *DoubleWrite) Write(path string, data []byte) {}
