package raftconsensus

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"time"
)

type Server struct {
	serverId    int
	C           *Consensus
	clients     map[int]*rpc.Client
	peerAddress map[int]string
}

func GetNewServer(id int, peerIds []int, peerAddress map[int]string) *Server {

	s := Server{}
	s.peerAddress = peerAddress
	c := Consensus{
		id:      id,
		peerIds: peerIds,
		server:  &s,
	}
	c.electionResetTime = time.Now()
	c.votedFor = -1

	s.C = &c
	s.clients = make(map[int]*rpc.Client)
	for k, v := range peerAddress {
		client, err := rpc.DialHTTP("tcp", v)
		if err != nil {
			log.Println("error dialing:", err)
		}
		s.clients[k] = client
	}

	rpc.Register(s.C)
	rpc.HandleHTTP()

	l, err := net.Listen("tcp", ":"+strings.Split(peerAddress[id], ":")[1])
	if err != nil {
		log.Fatal("listen error:", err)
	}
	go http.Serve(l, nil)

	return &s
}

func (s *Server) Serve() {

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
