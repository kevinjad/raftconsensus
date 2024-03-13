package raftconsensus

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu          sync.Mutex
	serverId    int
	C           *Consensus
	clients     map[int]*rpc.Client
	peerAddress map[int]string

	rpcServer *rpc.Server
}

func GetNewServer(id int, peerAddress map[int]string) *Server {
	s := Server{}
	s.serverId = id
	s.peerAddress = peerAddress
	s.clients = make(map[int]*rpc.Client)
	s.rpcServer = rpc.NewServer()
	peerIds := make([]int, 0, len(peerAddress))
	for k := range peerAddress {
		if s.serverId != k {
			peerIds = append(peerIds, k)
		}
	}

	c := NewConsensus(id, peerIds, &s)
	s.C = c
	return &s
}

func (s *Server) Serve() {
	s.mu.Lock()
	for k, v := range s.peerAddress {
		client, err := rpc.DialHTTP("tcp", v)
		if err != nil {
			log.Println("error dialing:", err)
		}
		s.clients[k] = client
	}

	s.rpcServer = rpc.NewServer()
	s.rpcServer.RegisterName("Consensus", s.C)
	s.rpcServer.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
	// log.Printf("%d %v", s.serverId, s.peerAddress)
	s.mu.Unlock()
	l, err := net.Listen("tcp", ":"+strings.Split(s.peerAddress[s.serverId], ":")[1])
	if err != nil {
		log.Fatal("listen error:", err)
	}
	go http.Serve(l, nil)

	go func() {
		c := s.C
		c.mu.Lock()
		c.electionResetTime = time.Now()
		c.mu.Unlock()
		c.RunElectionTimer()
	}()
}

func (s *Server) Call(peerId int, method string, arg interface{}, reply interface{}) error {
	clt := s.clients[peerId]
	var err error
	if clt == nil {
		clt, err = rpc.DialHTTP("tcp", s.peerAddress[peerId])
		if err != nil {
			log.Println("dialing:", err)
			return err
		}
		s.clients[peerId] = clt
	}
	return clt.Call(method, arg, reply)
}
