package execution

import "github.com/tuanta7/cataraft/internal/storage/disk"

// FlushPage enforces Logger-before-data when Logger is configured.
func (m *Manager) FlushPage(id disk.PageID) error {
	if m.buffer == nil {
		return nil
	}

	page, err := m.buffer.ReadPage(id)
	if err != nil {
		return err
	}

	if m.logger != nil && page.LSN() > m.logger.FlushedLSN() {
		if err = m.logger.FlushThrough(page.LSN()); err != nil {
			return err
		}
	}

	return m.buffer.Flush(id)
}

func (m *Manager) Write() ([]byte, error) {
	return nil, nil
}
