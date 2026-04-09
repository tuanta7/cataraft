package recovery

import "hash/crc32"

func Checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
