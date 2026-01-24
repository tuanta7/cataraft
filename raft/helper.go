package raft

import (
	"math/rand/v2"
	"time"
)

func RandomElectionTimeout() time.Duration {
	r := rand.Float64() + 1
	return time.Duration(150*r) * time.Millisecond
}
