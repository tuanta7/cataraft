package buffer

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

type DiskAdapter struct {
	baseDir     string
	direct      bool
	openedFiles map[string]*os.File
}

func NewDiskAdapter(baseDir string, direct bool) (*DiskAdapter, error) {
	_, err := os.Stat(baseDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(baseDir, 0755)
		if err != nil {
			return nil, err
		}
	}

	return &DiskAdapter{
		baseDir:     baseDir,
		direct:      direct,
		openedFiles: make(map[string]*os.File),
	}, nil
}

func (m *DiskAdapter) OpenFile(fn string) (*os.File, error) {
	path := filepath.Join(m.baseDir, fn)
	flags := os.O_RDWR | os.O_CREATE
	if m.direct {
		flags = flags | syscall.O_DIRECT
	}

	return os.OpenFile(path, flags, 0644)
}

func (m *DiskAdapter) CloseFile(fn string) error {
	path := filepath.Join(m.baseDir, fn)
	return m.openedFiles[path].Close()
}

func (m *DiskAdapter) Close() error {
	var errs error
	for _, file := range m.openedFiles {
		err := file.Close()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// ReadPage reads a page of data from the file associated with the given PageID into the provided page.
func (m *DiskAdapter) ReadPage(id PageID, page []byte) error {
	file, ok := m.openedFiles[id.fileName]
	if !ok {
		return os.ErrNotExist
	}

	_, err := file.ReadAt(page, id.offset())
	if err != nil {
		return err
	}

	return nil
}

func (m *DiskAdapter) WritePage(id PageID, page []byte) error {
	file, ok := m.openedFiles[id.fileName]
	if !ok {
		return os.ErrNotExist
	}

	_, err := file.WriteAt(page, id.offset())
	if err != nil {
		return err
	}

	return nil
}
