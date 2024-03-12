package raftconsensus

import "testing"

// TESTS
func TestRunElection(t *testing.T) {
	c := new(Consensus)
	c.peerIds = append(c.peerIds, 1)
	c.RunElectionTimer()
}
