package cataraft

import "github.com/tuanta7/cataraft/storage"

type Cataraft struct {
	storage storage.Engine
}
