package raftconsensus

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type ConsensusState int

const (
	Follower ConsensusState = iota
	Candidate
	Leader
	Dead
)

const DEBUG = 1

func (s ConsensusState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	case Dead:
		return "Dead"
	default:
		panic("unreachable")
	}
}

type LogEntry struct {
	Command interface{}
	TermId  int
}
type Consensus struct {
	id      int
	peerIds []int
	server  *Server

	mu sync.Mutex

	currentTerm       int
	votedFor          int
	state             ConsensusState
	electionResetTime time.Time

	log []LogEntry
}

func NewConsensus(id int, peerIds []int, server *Server) *Consensus {
	c := Consensus{
		id:       id,
		peerIds:  peerIds,
		server:   server,
		votedFor: -1,
		state:    Follower,
	}

	return &c
}

func (c *Consensus) debuglog(format string, args ...interface{}) {
	if DEBUG == 1 {
		format = fmt.Sprintf("[%d]", c.id) + format
		log.Printf(format, args...)
	}
}

func (c *Consensus) GetState() string {
	c.mu.Lock()
	state := c.state.String()
	c.mu.Unlock()
	return state
}

func (c *Consensus) startElection() {
	c.state = Candidate
	c.currentTerm += 1
	c.electionResetTime = time.Now()
	savedTerm := c.currentTerm
	votes := 1
	c.debuglog("becomes Candidate (currentTerm=%d); log=%v", savedTerm, c.log)

	for _, peerId := range c.peerIds {
		go func(peerId int) {
			args := RequestVouteArgs{
				Term:        savedTerm,
				CandidateId: c.id,
			}
			c.debuglog("sending RequestVote to %d: %+v", peerId, args)
			var reply RequestVoteReply
			err := c.server.Call(peerId, "Consensus.RequestVote", args, &reply)
			c.mu.Lock()
			defer c.mu.Unlock()
			if err == nil {

				if c.state != Candidate {
					c.debuglog("while waiting for reply state became: %s", c.state.String())
					return
				}

				if reply.Term > savedTerm {
					c.debuglog("found a greater term, so skipping wanna be leader phase")
					c.becomeFollower(reply.Term)
					return
				} else if reply.Term <= savedTerm {
					if reply.VoteGranted {
						votes++
						if 2*votes >= len(c.peerIds)+1 {
							c.debuglog("wins election with %d votes", votes)
							c.startLeader()
							return
						}
					}
				}
			} else {
				c.debuglog("Error log in startElection: " + err.Error())
			}
		}(peerId)
	}
}

func (c *Consensus) startLeader() {
	c.state = Leader
	c.debuglog("becoming a leader with term: %d and log: %v", c.currentTerm, c.log)

	go func() {
		//send periodic hearbeat to all peers
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			<-ticker.C
			c.sendHeartBeat()
			c.mu.Lock()
			if c.state != Leader {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		}
	}()
}

func (c *Consensus) sendHeartBeat() {
	c.mu.Lock()
	term := c.currentTerm
	c.mu.Unlock()

	args := AppendEntriesArg{
		Term:     term,
		LeaderId: c.id,
	}
	for _, peerId := range c.peerIds {
		c.debuglog("sending append entries to peer %d , args: ", peerId, args)
		var reply AppendEntriesReply
		go func(peerId int) {
			err := c.server.Call(peerId, "Consensus.AppendEntries", args, &reply)
			if err == nil {
				c.mu.Lock()
				defer c.mu.Unlock()
				if reply.Term > term {
					c.debuglog("reply term is greater in heart beat reply")
					c.becomeFollower(reply.Term)
					return
				}
			}

		}(peerId)
	}
}

func (c *Consensus) becomeFollower(term int) {
	c.debuglog("becomes Follower with term=%d; log=%v", term, c.log)
	c.state = Follower
	c.currentTerm = term
	c.votedFor = -1
	c.electionResetTime = time.Now()
	go c.RunElectionTimer()
}

func (c *Consensus) getElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (c *Consensus) RunElectionTimer() {
	timeoutDuration := c.getElectionTimeout()
	c.mu.Lock()
	termStarted := c.currentTerm
	c.mu.Unlock()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C
		c.mu.Lock()

		if c.state.String() != "Follower" && c.state.String() != "Candidate" {
			c.debuglog("In election timer, state: %s, bailing out", c.state.String())
			c.mu.Unlock()
			return
		}

		if termStarted != c.currentTerm {
			c.debuglog("In election timer, term changed from %d to %d", termStarted, c.currentTerm)
			c.mu.Unlock()
			return
		}

		elapsedTime := time.Since(c.electionResetTime)
		if elapsedTime >= timeoutDuration {
			c.startElection()
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

func (c *Consensus) RequestVote(args RequestVouteArgs, reply *RequestVoteReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == Dead {
		return nil
	}
	c.debuglog("RequestVote: %+v [currentTerm=%d, votedFor=%d]", args, c.currentTerm, c.votedFor)
	if args.Term > c.currentTerm {
		c.debuglog("request vote term is greater")
		c.becomeFollower(args.Term)
	}
	if args.Term == c.currentTerm && (c.votedFor == -1 || c.votedFor == args.CandidateId) {
		reply.VoteGranted = true
		c.votedFor = args.CandidateId
		c.electionResetTime = time.Now()
	} else {
		reply.VoteGranted = false
	}
	reply.Term = c.currentTerm
	c.debuglog("... RequestVote reply: %+v", reply)
	return nil
}

func (c *Consensus) AppendEntries(arg AppendEntriesArg, reply *AppendEntriesReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == Dead {
		return nil
	}
	c.debuglog("AppendEntries: %+v", arg)

	if arg.Term > c.currentTerm {
		c.debuglog("request term is greater in AppendEntries")
		c.becomeFollower(arg.Term)
	}

	reply.Success = false
	if arg.Term == c.currentTerm {
		if c.state != Follower {
			c.becomeFollower(arg.Term)
		}
		c.electionResetTime = time.Now()
		reply.Success = true
	}
	reply.Term = c.currentTerm
	c.debuglog("AppendEntries reply: %+v", *reply)
	return nil
}
