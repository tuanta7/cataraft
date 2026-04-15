package disk

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Adapter struct {
	mu          sync.RWMutex
	baseDir     string
	openedFiles map[string]*os.File
}

func NewAdapter(dataDir string) (*Adapter, error) {
	if dataDir == "" {
		return nil, errors.New("base directory is required")
	}

	baseDir := filepath.Clean(dataDir)

	info, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(baseDir, 0755); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", baseDir)
	}

	return &Adapter{
		baseDir:     baseDir,
		openedFiles: make(map[string]*os.File),
	}, nil
}

// ReadPage reads a page of data from the file associated with the given PageID into the provided page.
func (m *Adapter) ReadPage(id PageID) (*Page, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	page := NewPage(id)
	file, err := m.openFile(id.fileName)
	if err != nil {
		return nil, err
	}

	n, err := file.ReadAt(page.data, id.offset())
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		for i := n; i < len(page.data); i++ {
			page.data[i] = 0
		}
		return page, nil
	} else if err != nil {
		return nil, err
	}

	return page, nil
}

func (m *Adapter) WritePage(id PageID, page *Page) error {
	if page == nil {
		return errors.New("page cannot be nil")
	}
	if err := id.Validate(); err != nil {
		return err
	}

	file, err := m.openFile(id.fileName)
	if err != nil {
		return err
	}

	page.id = id
	n, err := file.WriteAt(page.data, id.offset())
	if err != nil {
		return err
	}
	if n != len(page.data) {
		return io.ErrShortWrite
	}

	page.isDirty = false
	return nil
}

func (m *Adapter) SyncFile(fn string) error {
	file, err := m.openFile(fn)
	if err != nil {
		return err
	}

	return file.Sync()
}

func (m *Adapter) Sync() error {
	m.mu.RLock()
	files := make([]*os.File, 0, len(m.openedFiles))
	for _, f := range m.openedFiles {
		files = append(files, f)
	}
	m.mu.RUnlock()

	var errs error
	for _, file := range files {
		if err := file.Sync(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
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

func (m *Adapter) FileSize(fn string) (int64, error) {
	file, err := m.openFile(fn)
	if err != nil {
		return 0, err
	}

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

func (m *Adapter) ReadFileAt(fn string, offset int64, buf []byte) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("invalid read offset %d", offset)
	}

	file, err := m.openFile(fn)
	if err != nil {
		return 0, err
	}

	return file.ReadAt(buf, offset)
}

func (m *Adapter) AppendFile(fn string, data []byte) (int64, error) {
	file, err := m.openFile(fn)
	if err != nil {
		return 0, err
	}

	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	n, err := file.Write(data)
	if err != nil {
		return 0, err
	}
	if n != len(data) {
		return 0, io.ErrShortWrite
	}

	return offset, nil
}

func (m *Adapter) openFile(fn string) (*os.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if f, ok := m.openedFiles[fn]; ok {
		return f, nil
	}

	path, err := m.filePath(fn)
	if err != nil {
		return nil, err
	}

	flags := os.O_RDWR | os.O_CREATE

	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, err
	}

	m.openedFiles[fn] = f
	return f, nil
}

func (m *Adapter) filePath(fn string) (string, error) {
	if fn == "" {
		return "", errors.New("file name is required")
	}

	cleanName := filepath.Clean(fn)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file name %q", fn)
	}

	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("absolute file name %q is not allowed", fn)
	}

	path := filepath.Join(m.baseDir, cleanName)

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}

	return path, nil
}
