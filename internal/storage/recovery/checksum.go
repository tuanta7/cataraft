package recovery

import "hash/crc32"

func checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
