package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"goDistributedSystemDemo/multipaxos_kvstore/persistence"
	"goDistributedSystemDemo/multipaxos_kvstore/types"
)

// ServerState represents the current state of the server
// type ServerState int

// const (
// 	Follower ServerState = iota
// 	Candidate
// 	Leader
// )

// func (s ServerState) String() string {
// 	switch s {
// 	case Follower:
// 		return "Follower"
// 	case Candidate:
// 		return "Candidate"
// 	case Leader:
// 		return "Leader"
// 	default:
// 		return "Unknown"
// 	}
// }

// PaxosServer implements all three Paxos roles:
// - Proposer (Leader): Drives log replication and initiates elections
// - Acceptor: Votes on proposals and persists state
// - Learner: Applies committed log entries to the KVStore
type PaxosServer struct {
	mu sync.RWMutex

	// Server identity
	serverID int
	peers    []string // Addresses of all servers (including self)

	// Current state
	state types.ServerState

	// Persistent state (persisted to disk)
	ps *persistence.PersistentState

	// Volatile state
	currentLeader int
	commitIndex   int           // Highest log entry known to be committed
	lastApplied   int           // Highest log entry applied to state machine
	kvStore       *types.KVStore // The state machine

	// Leader state (reinitialized after election)
	nextIndex  map[int]int // For each server, index of next log entry to send
	matchIndex map[int]int // For each server, index of highest log entry known to be replicated

	// Election state
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration
	lastHeartbeat    time.Time

	// Channels for coordination
	heartbeatCh chan bool
	stopCh      chan bool

	// RPC clients for communication with peers
	peerClients map[int]*PaxosClient
}

// NewPaxosServer creates a new Paxos server
func NewPaxosServer(serverID int, peers []string, dataDir string, LeaderRole bool) (*PaxosServer, error) {
	// Create persistent state
	ps, err := persistence.NewPersistentState(serverID, dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent state: %v", err)
	}

	server := &PaxosServer{
		serverID:         serverID,
		peers:            peers,
		state:            types.Follower,
		// state:            types.Default,
		ps:               ps,
		currentLeader:    -1,
		commitIndex:      -1,
		lastApplied:      -1,
		kvStore:          types.NewKVStore(),
		nextIndex:        make(map[int]int),
		matchIndex:       make(map[int]int),
		electionTimeout:  time.Duration(150+serverID*50) * time.Millisecond,
		heartbeatTimeout: 500 * time.Millisecond,
		lastHeartbeat:    time.Now(),
		heartbeatCh:      make(chan bool, 10),
		stopCh:           make(chan bool),
		peerClients:      make(map[int]*PaxosClient),
	}

	if LeaderRole {
		server.state = types.Leader
		server.currentLeader = serverID
		server.ps.Npromised = types.NewBallotNumber(1, serverID)
		log.Printf("Server %d: Starting as LEADER", serverID)
	}

	return server, nil
}

// Start begins the server's main event loop
func (s *PaxosServer) Start() {
	log.Printf("Server %d started as %s", s.serverID, s.state)
	go s.electionLoop()
	go s.applyLoop()
}

// Stop gracefully shuts down the server
func (s *PaxosServer) Stop() {
	close(s.stopCh)
}

// electionLoop handles leader election and heartbeat timeouts
func (s *PaxosServer) electionLoop() {
	ticker := time.NewTicker(s.heartbeatTimeout)
	defer ticker.Stop()
	s.lastHeartbeat = time.Now()
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.heartbeatCh:
			s.mu.Lock()
			s.lastHeartbeat = time.Now()
			s.mu.Unlock()
		case <-ticker.C:
			s.mu.Lock()
			state := s.state
			timeSinceHeartbeat := time.Since(s.lastHeartbeat)
			s.mu.Unlock()

			// If we're a follower and haven't heard from leader, start election
			if state == types.Follower && timeSinceHeartbeat > s.heartbeatTimeout*3 {
				log.Printf("Server %d: Election timeout, starting election", s.serverID)
				s.startElection()
			}

			// If we're a leader, send heartbeats
			if state == types.Leader {
				s.sendHeartbeats()
			}
		}
	}
}

// applyLoop applies committed log entries to the state machine
// This is the Learner role
func (s *PaxosServer) applyLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.applyCommittedEntries()
		}
	}
}

// applyCommittedEntries applies all committed but not yet applied log entries
// Learner Instruction: Must apply entries strictly in sequential order
func (s *PaxosServer) applyCommittedEntries() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.lastApplied < s.commitIndex {
		s.lastApplied++
		entry, exists := s.ps.GetLogEntry(s.lastApplied)
		if !exists {
			log.Printf("Server %d: ERROR - Log entry %d does not exist", s.serverID, s.lastApplied)
			break
		}

		// Apply to state machine
		result := s.kvStore.Apply(entry)
		log.Printf("Server %d: Applied entry %d: %s -> Result: %s",
			s.serverID, s.lastApplied, entry.String(), result)
	}
}

// ============================================================================
// ACCEPTOR ROLE - Phase 1 (Prepare/Promise)
// ============================================================================

// HandlePrepare handles Phase 1: Prepare RPC from a Candidate
// Acceptor logic: Vote & Persist
func (s *PaxosServer) HandlePrepare(ctx context.Context, ballot types.BallotNumber) (bool, types.BallotNumber, []types.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	npromised := s.ps.GetNpromised()

	// Check if this ballot is higher than what we've promised
	if ballot.GreaterThan(npromised) {
		// Persist the new promise before responding
		if err := s.ps.SetNpromised(ballot); err != nil {
			return false, types.BallotNumber{}, nil, fmt.Errorf("failed to persist Npromised: %v", err)
		}

		// Send back previously accepted log entries for safety reconciliation
		acceptedEntries := s.ps.GetAllAcceptedEntries()

		log.Printf("Server %d: Promised to ballot %s (previous: %s), returning %d accepted entries",
			s.serverID, ballot.String(), npromised.String(), len(acceptedEntries))

		// Reset to follower and update heartbeat
		s.state = types.Follower
		s.lastHeartbeat = time.Now()

		return true, ballot, acceptedEntries, nil
	}

	log.Printf("Server %d: Rejected Prepare from ballot %s (promised: %s)",
		s.serverID, ballot.String(), npromised.String())

	return false, npromised, nil, nil
}

// ============================================================================
// ACCEPTOR ROLE - Phase 2 (Accept/Accepted)
// ============================================================================

// HandleAccept handles Phase 2: Accept RPC from the Leader
// Acceptor logic: Persist LogEntry before acknowledging
func (s *PaxosServer) HandleAccept(ctx context.Context, ballot types.BallotNumber, entry types.LogEntry) (bool, int, types.BallotNumber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	npromised := s.ps.GetNpromised()

	// Check if ballot is at least as high as what we've promised
	if ballot.Compare(npromised) >= 0 {
		// Persist the log entry
		if err := s.ps.UpdateLog(entry.Index, entry); err != nil {
			return false, entry.Index, types.BallotNumber{}, fmt.Errorf("failed to persist log entry: %v", err)
		}

		// Persist the highest accepted ballot for this index
		if err := s.ps.SetHighestAcceptedBallot(entry.Index, ballot); err != nil {
			return false, entry.Index, types.BallotNumber{}, fmt.Errorf("failed to persist highest accepted ballot: %v", err)
		}

		log.Printf("Server %d: Accepted entry %d with ballot %s",
			s.serverID, entry.Index, ballot.String())

		// Update heartbeat
		s.lastHeartbeat = time.Now()

		return true, entry.Index, ballot, nil
	}

	log.Printf("Server %d: Rejected Accept for entry %d from ballot %s (promised: %s)",
		s.serverID, entry.Index, ballot.String(), npromised.String())

	return false, entry.Index, npromised, nil
}

// ============================================================================
// PROPOSER ROLE - Phase 1 (Leader Election)
// ============================================================================

// startElection initiates a new leader election (Phase 1)
func (s *PaxosServer) startElection() {
	s.mu.Lock()
	s.state = types.Candidate

	// Increment term number
	currentBallot := s.ps.GetNpromised()
	newBallot := types.NewBallotNumber(currentBallot.TermNumber+1, s.serverID)

	// Vote for ourselves
	if err := s.ps.SetNpromised(newBallot); err != nil {
		log.Printf("Server %d: Failed to persist new ballot: %v", s.serverID, err)
		s.state = types.Follower
		s.mu.Unlock()
		return
	}

	log.Printf("Server %d: Starting election with ballot %s", s.serverID, newBallot.String())
	s.mu.Unlock()

	// Send Prepare RPCs to all peers
	votes := 1 // Vote for ourselves
	var voteMu sync.Mutex
	var wg sync.WaitGroup

	for i, peerAddr := range s.peers {
		if i == s.serverID {
			continue // Skip ourselves
		}

		wg.Add(1)
		go func(peerID int, addr string) {
			defer wg.Done()

			client, err := s.getPeerClient(peerID, addr)
			if err != nil {
				log.Printf("Server %d: Failed to get client for peer %d: %v", s.serverID, peerID, err)
				return
			}

			promise, _, acceptedEntries, err := client.Prepare(context.Background(), newBallot)
			if err != nil {
				log.Printf("Server %d: Prepare to peer %d failed: %v", s.serverID, peerID, err)
				return
			}

			if promise {
				voteMu.Lock()
				votes++
				currentVotes := votes
				voteMu.Unlock()

				log.Printf("Server %d: Received Promise from peer %d (votes: %d, accepted entries: %d)",
					s.serverID, peerID, currentVotes, len(acceptedEntries))

				// Store accepted entries for state adoption
				s.adoptAcceptedEntries(acceptedEntries)
			}
		}(i, peerAddr)
	}

	wg.Wait()

	// Check if we won the election (majority)
	majority := len(s.peers)/2 + 1
	if votes >= majority {
		s.becomeLeader(newBallot)
	} else {
		s.mu.Lock()
		s.state = types.Follower
		s.mu.Unlock()
		log.Printf("Server %d: Election failed, received %d votes (need %d)", s.serverID, votes, majority)
	}
}

// adoptAcceptedEntries adopts previously accepted entries from Promise responses
// This is crucial for safety - the new leader must adopt uncommitted values
func (s *PaxosServer) adoptAcceptedEntries(entries []types.LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range entries {
		existing, exists := s.ps.GetLogEntry(entry.Index)
		if !exists || entry.BallotNum.GreaterThan(existing.BallotNum) {
			// Adopt this entry (will be re-proposed with new ballot number)
			s.ps.UpdateLog(entry.Index, entry)
			log.Printf("Server %d: Adopted entry %d from Promise response", s.serverID, entry.Index)
		}
	}
}

// becomeLeader transitions the server to Leader state
func (s *PaxosServer) becomeLeader(ballot types.BallotNumber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = types.Leader
	s.currentLeader = s.serverID
	logLength := s.ps.GetLogLength()

	// Initialize leader state
	for i := range s.peers {
		s.nextIndex[i] = logLength
		s.matchIndex[i] = -1
	}

	log.Printf("Server %d: Became LEADER with ballot %s", s.serverID, ballot.String())

	// Immediately send a No-Op entry to commit previous entries
	go s.proposeNoOp(ballot)
}

// proposeNoOp proposes a No-Op entry to commit any uncommitted entries
func (s *PaxosServer) proposeNoOp(ballot types.BallotNumber) {
	logLength := s.ps.GetLogLength()
	entry := types.NewLogEntry(logLength, ballot, types.OpNoOp, "", "")

	log.Printf("Server %d: Proposing No-Op entry at index %d", s.serverID, logLength)
	s.proposeEntry(entry)
}

// ============================================================================
// PROPOSER ROLE - Phase 2 (Log Replication)
// ============================================================================

// proposeEntry proposes a new log entry (Phase 2)
func (s *PaxosServer) proposeEntry(entry types.LogEntry) error {
	s.mu.Lock()
	if s.state != types.Leader {
		s.mu.Unlock()
		return fmt.Errorf("not the leader")
	}

	ballot := s.ps.GetNpromised()
	entry.BallotNum = ballot

	// Append to our own log first
	if err := s.ps.AppendLog(entry); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to append to log: %v", err)
	}

	log.Printf("Server %d: Proposing entry %d: %s", s.serverID, entry.Index, entry.String())
	s.mu.Unlock()

	// Send Accept RPCs to all peers
	accepts := 1 // Accept from ourselves
	var acceptMu sync.Mutex
	var wg sync.WaitGroup

	for i, peerAddr := range s.peers {
		if i == s.serverID {
			continue
		}

		wg.Add(1)
		go func(peerID int, addr string) {
			defer wg.Done()

			client, err := s.getPeerClient(peerID, addr)
			if err != nil {
				log.Printf("Server %d: Failed to get client for peer %d: %v", s.serverID, peerID, err)
				return
			}

			accepted, _, _, err := client.Accept(context.Background(), ballot, entry)
			if err != nil {
				log.Printf("Server %d: Accept to peer %d failed: %v", s.serverID, peerID, err)
				return
			}

			if accepted {
				acceptMu.Lock()
				accepts++
				currentAccepts := accepts
				acceptMu.Unlock()

				log.Printf("Server %d: Received Accepted from peer %d for entry %d (accepts: %d)",
					s.serverID, peerID, entry.Index, currentAccepts)
			}
		}(i, peerAddr)
	}

	wg.Wait()

	// Check if we have majority
	majority := len(s.peers)/2 + 1
	if accepts >= majority {
		// Commit the entry
		s.mu.Lock()
		if entry.Index > s.commitIndex {
			s.commitIndex = entry.Index
			log.Printf("Server %d: Committed entry %d (accepts: %d/%d)",
				s.serverID, entry.Index, accepts, len(s.peers))
		}
		s.mu.Unlock()
		return nil
	}

	// Failed to get majority - likely lost leadership
	s.mu.Lock()
	if s.state == types.Leader {
		log.Printf("Server %d: Failed to get majority accepts (%d/%d), stepping down from Leader",
			s.serverID, accepts, len(s.peers))
		s.state = types.Follower
		s.currentLeader = -1
	}
	s.mu.Unlock()

	return fmt.Errorf("failed to get majority accepts (%d/%d)", accepts, len(s.peers))
}

// sendHeartbeats sends periodic heartbeats to maintain leadership
func (s *PaxosServer) sendHeartbeats() {
	s.mu.RLock()
	if s.state != types.Leader {
		s.mu.RUnlock()
		return
	}
	ballot := s.ps.GetNpromised()
	commitIndex := s.commitIndex
	leaderLogLength := s.ps.GetLogLength()
	s.mu.RUnlock()

	// Track successful heartbeats
	successCount := 1 // Count ourselves
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, peerAddr := range s.peers {
		if i == s.serverID {
			continue
		}

		wg.Add(1)
		go func(peerID int, addr string) {
			defer wg.Done()

			client, err := s.getPeerClient(peerID, addr)
			if err != nil {
				return
			}

			success, peerLastLogEntryIndex, err := client.Heartbeat(context.Background(), ballot, commitIndex)
			if err != nil {
				log.Printf("Server %d: Heartbeat to peer %d failed: %v", s.serverID, peerID, err)
				return
			}

			if !success {
				return
			}

			// Count successful heartbeat
			mu.Lock()
			successCount++
			mu.Unlock()

			// Check if peer is behind and needs state transfer
			if peerLastLogEntryIndex < leaderLogLength {
				log.Printf("Server %d: Peer %d is behind (peer: %d, leader: %d), initiating state transfer",
					s.serverID, peerID, peerLastLogEntryIndex, leaderLogLength)
				s.initiateStateTransfer(peerID, addr, peerLastLogEntryIndex, leaderLogLength)
			}
		}(i, peerAddr)
	}

	wg.Wait()

	// Check if we can still reach majority
	majority := len(s.peers)/2 + 1
	if successCount < majority {
		s.mu.Lock()
		if s.state == types.Leader {
			log.Printf("Server %d: Lost contact with majority (heartbeat success: %d/%d), stepping down from Leader",
				s.serverID, successCount, len(s.peers))
			s.state = types.Follower
			s.currentLeader = -1
		}
		s.mu.Unlock()
	}
}

// initiateStateTransfer sends missing log entries to a lagging follower
func (s *PaxosServer) initiateStateTransfer(peerID int, peerAddr string, peerLogLength int, leaderLogLength int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Gather missing entries
	missingEntries := make([]types.LogEntry, 0, leaderLogLength-peerLogLength)
	for i := peerLogLength; i < leaderLogLength; i++ {
		entry, exists := s.ps.GetLogEntry(i)
		if !exists {
			log.Printf("Server %d: Warning - Missing log entry %d while preparing state transfer for peer %d",
				s.serverID, i, peerID)
			continue
		}
		missingEntries = append(missingEntries, entry)
	}

	if len(missingEntries) == 0 {
		return
	}

	log.Printf("Server %d: Sending %d missing entries to peer %d (indices %d to %d)",
		s.serverID, len(missingEntries), peerID, peerLogLength, leaderLogLength-1)

	// Send state transfer
	client, err := s.getPeerClient(peerID, peerAddr)
	if err != nil {
		log.Printf("Server %d: Failed to get client for peer %d during state transfer: %v",
			s.serverID, peerID, err)
		return
	}

	success, err := client.TransferState(context.Background(), missingEntries)
	if err != nil {
		log.Printf("Server %d: State transfer to peer %d failed: %v", s.serverID, peerID, err)
		return
	}

	if success {
		log.Printf("Server %d: State transfer to peer %d completed successfully", s.serverID, peerID)
	} else {
		log.Printf("Server %d: State transfer to peer %d returned failure", s.serverID, peerID)
	}
}

// getPeerClient returns or creates a client for a peer
func (s *PaxosServer) getPeerClient(peerID int, addr string) (*PaxosClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client, exists := s.peerClients[peerID]
	if !exists {
		var err error
		client, err = NewPaxosClient(addr)
		if err != nil {
			return nil, err
		}
		s.peerClients[peerID] = client
	}
	return client, nil
}

// ============================================================================
// CLIENT OPERATIONS
// ============================================================================

// Put handles a client Put operation
func (s *PaxosServer) Put(key, value string) (bool, string, error) {
	s.mu.RLock()
	if s.state != types.Leader {
		leader := s.currentLeader
		s.mu.RUnlock()
		return false, fmt.Sprintf("not the leader (leader: %d)", leader), fmt.Errorf("not the leader")
	}
	ballot := s.ps.GetNpromised()
	logLength := s.ps.GetLogLength()
	s.mu.RUnlock()

	if value == "reject" {
		return false, "simulated rejection of value 'reject'", fmt.Errorf("simulated rejection")
	}
	entry := types.NewLogEntry(logLength, ballot, types.OpPut, key, value)
	if err := s.proposeEntry(entry); err != nil {
		return false, err.Error(), err
	}

	return true, "OK", nil
}

// Get handles a client Get operation
func (s *PaxosServer) Get(key string) (bool, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, exists := s.kvStore.Get(key)
	if !exists {
		return false, "", fmt.Errorf("key not found")
	}

	return true, val, nil
}

// Delete handles a client Delete operation
func (s *PaxosServer) Delete(key string) (bool, string, error) {
	s.mu.RLock()
	if s.state != types.Leader {
		leader := s.currentLeader
		s.mu.RUnlock()
		return false, fmt.Sprintf("not the leader (leader: %d)", leader), fmt.Errorf("not the leader")
	}
	ballot := s.ps.GetNpromised()
	logLength := s.ps.GetLogLength()
	s.mu.RUnlock()

	entry := types.NewLogEntry(logLength, ballot, types.OpDelete, key, "")
	if err := s.proposeEntry(entry); err != nil {
		return false, err.Error(), err
	}

	return true, "OK", nil
}

// HandleHeartbeat handles heartbeat from leader
// func (s *PaxosServer) HandleHeartbeat(ballot types.BallotNumber, commitIndex int) bool {
func (s *PaxosServer) HandleHeartbeat(ballot types.BallotNumber, commitIndex int) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	npromised := s.ps.GetNpromised()

	if ballot.Compare(npromised) >= 0 {
		s.state = types.Follower
		s.currentLeader = ballot.ServerID
		s.lastHeartbeat = time.Now()

		// Update commit index if leader's is higher
		if commitIndex > s.commitIndex {
			s.commitIndex = commitIndex
		}

		select {
		case s.heartbeatCh <- true:
		default:
		}

		return true, s.ps.GetLogLength()
	}

	return false, s.ps.GetLogLength()
}

// HandleStateTransfer handles state transfer from leader
func (s *PaxosServer) HandleStateTransfer(ctx context.Context, missingEntries []types.LogEntry) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Server %d: Receiving state transfer with %d missing entries", s.serverID, len(missingEntries))

	// Persist each missing entry to the log
	for _, entry := range missingEntries {
		// Check if we already have this entry
		existing, exists := s.ps.GetLogEntry(entry.Index)
		if exists && existing.BallotNum.Compare(entry.BallotNum) >= 0 {
			// We already have this entry or a newer one, skip it
			log.Printf("Server %d: Skipping entry %d (already exists with ballot %s)",
				s.serverID, entry.Index, existing.BallotNum.String())
			continue
		}

		// Persist the log entry
		if err := s.ps.UpdateLog(entry.Index, entry); err != nil {
			log.Printf("Server %d: Failed to persist entry %d during state transfer: %v",
				s.serverID, entry.Index, err)
			return false, fmt.Errorf("failed to persist log entry: %v", err)
		}

		// Persist the highest accepted ballot for this index
		if err := s.ps.SetHighestAcceptedBallot(entry.Index, entry.BallotNum); err != nil {
			log.Printf("Server %d: Failed to persist ballot for entry %d during state transfer: %v",
				s.serverID, entry.Index, err)
			return false, fmt.Errorf("failed to persist highest accepted ballot: %v", err)
		}

		log.Printf("Server %d: Applied entry %d from state transfer: %s",
			s.serverID, entry.Index, entry.String())
	}

	log.Printf("Server %d: State transfer completed successfully", s.serverID)
	return true, nil
}

// GetState returns current server state (for debugging)
func (s *PaxosServer) GetState() (types.ServerState, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, s.commitIndex, s.lastApplied
}
