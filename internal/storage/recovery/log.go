package recovery

type LogRecord struct{}

type WriteAheadLog struct{}

func (w *WriteAheadLog) Write(path string, data []byte) {}
