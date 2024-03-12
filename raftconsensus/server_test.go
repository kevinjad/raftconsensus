package raftconsensus

import (
	"testing"
	"time"
)

// TESTS
func TestServe(t *testing.T) {
	mapOfAddress := map[int]string{
		1: "127.0.0.1:8080",
		2: "127.0.0.1:8081",
		3: "127.0.0.1:8082",
		4: "127.0.0.1:8083",
	}
	servers := []*Server{}
	for k := range mapOfAddress {
		//prepare peer lsit
		servers = append(servers, GetNewServer(k, mapOfAddress))
	}
	for _, s := range servers {
		s.Serve()
		go s.C.RunElectionTimer()
		time.Sleep(1 * time.Second)
	}

	time.Sleep(50 * time.Second)
}
