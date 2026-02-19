package disk

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type Adapter struct {
	mu          sync.RWMutex
	baseDir     string
	direct      bool
	openedFiles map[string]*os.File
}

func NewAdapter(baseDir string, direct bool) (*Adapter, error) {
	_, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(baseDir, 0755)
		if err != nil {
			return nil, err
		}
	}

	return &Adapter{
		baseDir:     baseDir,
		direct:      direct,
		openedFiles: make(map[string]*os.File),
	}, nil
}

func (m *Adapter) openFile(fn string) (*os.File, error) {
	// use write lock to prevent concurrent file opens
	m.mu.Lock()
	defer m.mu.Unlock()

	if f, ok := m.openedFiles[fn]; ok {
		return f, nil
	}

	path := filepath.Join(m.baseDir, fn)
	flags := os.O_RDWR | os.O_CREATE
	if m.direct {
		flags = flags | syscall.O_DIRECT
	}

	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, err
	}

	m.openedFiles[fn] = f
	return f, nil
}

func (m *Adapter) CloseFile(fn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, ok := m.openedFiles[fn]
	if !ok {
		return os.ErrNotExist
	}
	delete(m.openedFiles, fn)

	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

func (m *Adapter) Close() error {
	m.mu.Lock()
	files := make([]*os.File, 0, len(m.openedFiles))
	for k, f := range m.openedFiles {
		files = append(files, f)
		delete(m.openedFiles, k)
	}
	m.mu.Unlock()

	var errs error
	for _, file := range files {
		if err := file.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// ReadPage reads a page of data from the file associated with the given PageID into the provided page.
func (m *Adapter) ReadPage(id PageID) (*Page, error) {
	m.mu.RLock()
	file, ok := m.openedFiles[id.fileName]
	m.mu.RUnlock()

	page := NewPage(id)

	if ok {
		_, err := file.ReadAt(page.data, id.offset())
		return page, err
	}

	file, err := m.openFile(id.fileName)
	if err != nil {
		return nil, err
	}

	_, err = file.ReadAt(page.data, id.offset())
	return page, err
}

func (m *Adapter) WritePage(id PageID, page *Page) error {
	if page == nil {
		return errors.New("page cannot be nil")
	}

	file, err := m.openFile(id.fileName)
	if err != nil {
		return err
	}

	_, err = file.WriteAt(page.data, id.offset())
	return err
}
