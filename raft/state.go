package raft

type State struct {
	role        Role
	commitIndex uint64
	lastApplied uint64
	nextIndex   map[uint64]uint64
	matchIndex  map[uint64]uint64
}

func NewState(role Role) State {
	return State{
		role: role,
	}
}
