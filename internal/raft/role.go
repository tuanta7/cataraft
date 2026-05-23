package raft

import "context"

type Role string

const (
	RoleFollower  Role = "Follower"
	RoleCandidate Role = "Candidate"
	RoleLeader    Role = "Leader"
)

type Follower interface {
	Wait(ctx context.Context)
}

type Candidate interface {
	RequestVote(ctx context.Context)
}

type Leader interface {
	AppendEntries(ctx context.Context)
}
