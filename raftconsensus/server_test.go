package raftconsensus

import (
	"testing"
	"time"
)

// TESTS
func TestCall(t *testing.T) {
	idmap := map[int]bool{1: true, 2: true, 3: true, 4: true}
	mapOfAddress := map[int]string{1: "127.0.0.1:8080",
		2: "127.0.0.1:8081",
		3: "127.0.0.1:8082",
		4: "127.0.0.1:8083"}
	servers := []*Server{}

	listOfIds := make([]int, 0, len(idmap))
	for k := range idmap {
		listOfIds = append(listOfIds, k)
	}

	for _, id := range listOfIds {
		keys := make([]int, 0, len(idmap))
		for k := range idmap {
			if k != id {
				keys = append(keys, k)
			}
		}

		servers = append(servers, GetNewServer(id, keys, mapOfAddress))
	}
	for _, server := range servers {
		go server.C.RunElectionTimer()
	}
	time.Sleep(50 * time.Second)
}
