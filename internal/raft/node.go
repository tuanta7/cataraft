package raft

import (
	"context"
	"time"

	"github.com/tuanta7/cataraft/pkg/randx"
	pbv1 "github.com/tuanta7/cataraft/proto/gopb/v1"
)

const (
	electionTimeout = 500 * time.Millisecond
)

type Node struct {
	id          uint64
	state       State
	currentTerm uint64
	votedFor    *uint64
	log         []LogEntry
	peers       []Peer
}

func NewNode(id uint64, peers ...Peer) *Node {
	return &Node{
		id:          id,
		state:       NewState(RoleFollower),
		currentTerm: 0,
		votedFor:    nil,
		log:         make([]LogEntry, 0),
		peers:       peers,
	}
}

func (n *Node) Wait(ctx context.Context) {}

func (n *Node) RequestVote(ctx context.Context) {
	req := pbv1.RequestVoteRequest{
		Term:         n.currentTerm,
		CandidateId:  n.id,
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	_ = req
}

func (n *Node) randomTimeoutForNextElection() {
	duration := randx.RandomDuration(150*time.Millisecond, 300*time.Millisecond)
	time.Sleep(duration)
}
