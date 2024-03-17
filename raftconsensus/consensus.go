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

type CommitEntry struct {
	Command interface{}
	Index   int
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

	log             []LogEntry
	peerIndex       map[int]int
	peerBeforeIndex map[int]int
	commitIndex     int
	lastApplied     int

	newCommitReadyChan chan struct{}
	commitChan         chan CommitEntry
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

func (c *Consensus) commitChanSender() {
	for range c.newCommitReadyChan {
		//okay we have a commitIndex change
		c.mu.Lock()
		la := c.lastApplied
		ci := c.commitIndex
		term := c.currentTerm
		var entries []LogEntry
		if ci > la {
			entries = c.log[la+1 : ci+1]
			c.lastApplied = ci
		}
		c.mu.Unlock()
		c.debuglog("commitChanSender entries=%v, savedLastApplied=%d", entries, la)

		for i, entry := range entries {
			ce := CommitEntry{
				Command: entry.Command,
				TermId:  term,
				Index:   la + i + 1,
			}
			c.commitChan <- ce
		}
	}
	c.debuglog("commitChanSender done")
}

func (c *Consensus) sendHeartBeat() {
	c.mu.Lock()
	term := c.currentTerm
	c.mu.Unlock()

	for _, peerId := range c.peerIds {
		go func(peerId int) {
			c.mu.Lock()
			latestIndex := c.peerIndex[peerId]
			prevIndex := latestIndex - 1
			prevLogTerm := -1
			if prevIndex >= 0 {
				prevLogTerm = c.log[prevIndex].TermId
			}
			entries := c.log[latestIndex:]

			args := AppendEntriesArg{
				Term:         term,
				LeaderId:     c.id,
				PrevLogIndex: prevIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      entries,
				LeaderCommit: c.commitIndex,
			}
			c.mu.Unlock()
			c.debuglog("sending append entries to peer %d , args: ", peerId, args)
			var reply AppendEntriesReply
			err := c.server.Call(peerId, "Consensus.AppendEntries", args, &reply)
			if err == nil {
				c.mu.Lock()
				defer c.mu.Unlock()
				if reply.Term > term {
					c.debuglog("reply term is greater in heart beat reply")
					c.becomeFollower(reply.Term)
					return
				}
				if c.state == Leader && term == reply.Term {
					if reply.Success {
						c.peerIndex[peerId] = latestIndex + len(entries)
						c.peerBeforeIndex[peerId] = c.peerIndex[peerId] - 1
						c.debuglog("AppendEntries reply from %d success: peerIndex := %v, peerBeforeIndex := %v", peerId, c.peerIndex, c.peerBeforeIndex)

						savedCommitIndex := c.commitIndex
						for i := c.commitIndex; i < len(c.log); i++ {
							if c.log[i].TermId == c.currentTerm {
								repcount := 1
								for _, pid := range c.peerIds {
									if c.peerBeforeIndex[pid] >= i {
										repcount++
									}
								}
								if 2*repcount > len(c.peerIds)+1 {
									c.commitIndex = i
								}
							}
						}
						if savedCommitIndex != c.commitIndex {
							c.debuglog("leader sets commitIndex := %d", c.commitIndex)
							c.newCommitReadyChan <- struct{}{}
						}
					}
				} else {
					c.peerIndex[peerId] = latestIndex - 1
					c.debuglog("AppendEntries reply from %d !success: peerIndex := %d", peerId, latestIndex-1)
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

func (c *Consensus) Submit(command interface{}) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.debuglog("Submit received by %v: %v", c.state, command)
	if c.state == Leader {
		logEntry := LogEntry{
			Command: command,
			TermId:  c.currentTerm,
		}
		c.log = append(c.log, logEntry)
		return true
	} else {
		return false
	}
}
