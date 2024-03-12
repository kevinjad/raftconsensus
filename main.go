package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/kevinjad/raftconsensus/raftconsensus"
)

func main() {
	source, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("Provide source properly")
	}
	mapOfAddress := map[int]string{
		1: "127.0.0.1:8080",
		2: "127.0.0.1:8081",
		3: "127.0.0.1:8082",
		4: "127.0.0.1:8083",
	}

	peerId := []int{}
	for k := range mapOfAddress {
		if source != k {
			peerId = append(peerId, k)
		}
	}
	s := raftconsensus.GetNewServer(source, peerId, mapOfAddress)
	s.C.RunElectionTimer()
	time.Sleep(50 * time.Second)
}
