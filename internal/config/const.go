package config

const (
	PageSize = 8192 // 8KB

	WALLengthFieldSize   = 4 // 4 bytes
	WALChecksumFieldSize = 4 // 4 bytes
	WALHeaderSize        = WALLengthFieldSize + WALChecksumFieldSize

	RootTableFileName = "root.table"
)
