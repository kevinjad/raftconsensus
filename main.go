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

	s := raftconsensus.GetNewServer(source, mapOfAddress)
	s.Serve()
	s.C.RunElectionTimer()
	time.Sleep(3 * time.Second)
	log.Println(s.C.GetState())
	time.Sleep(50 * time.Second)
}
