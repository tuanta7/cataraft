package disk

import (
	"errors"
	"fmt"

	"github.com/tuanta7/cataraft/internal/config"
)

type PageID struct {
	fileName string
	pageNum  int64
}

func NewPageID(fileName string, pageNum int64) PageID {
	return PageID{
		fileName: fileName,
		pageNum:  pageNum,
	}
}

func (i PageID) Validate() error {
	if i.fileName == "" {
		return errors.New("file name is required")
	}

	if i.pageNum < 0 {
		return fmt.Errorf("invalid page number %d", i.pageNum)
	}

	return nil
}

func (i PageID) FileName() string {
	return i.fileName
}

func (i PageID) PageNum() int64 {
	return i.pageNum
}

func (i PageID) offset() int64 {
	return i.pageNum * config.PageSize
}
