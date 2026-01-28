package raft

type LogEntry struct {
	Term uint64
	Data []byte
}
