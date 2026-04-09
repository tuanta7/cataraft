package writer

import "github.com/tuanta7/cataraft/internal/storage/disk"

func ClonePage(id disk.PageID, page *disk.Page) (*disk.Page, error) {
	cloned := disk.NewPage(id)
	cloned.SetLSN(page.LSN())
	if err := cloned.Reset(page.Data()); err != nil {
		return nil, err
	}

	return cloned, nil
}
