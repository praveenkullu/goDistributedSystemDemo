package types

import "fmt"

// BallotNumber represents (Term Number, Server ID)
// Used to establish priority in Paxos protocol
type BallotNumber struct {
	TermNumber int
	ServerID   int
}

// NewBallotNumber creates a new ballot number
func NewBallotNumber(termNumber, serverID int) BallotNumber {
	return BallotNumber{
		TermNumber: termNumber,
		ServerID:   serverID,
	}
}

// Compare returns:
// -1 if this ballot is less than other
//  0 if equal
//  1 if this ballot is greater than other
func (b BallotNumber) Compare(other BallotNumber) int {
	if b.TermNumber < other.TermNumber {
		return -1
	}
	if b.TermNumber > other.TermNumber {
		return 1
	}
	// Term numbers are equal, use server ID as tie-breaker
	if b.ServerID < other.ServerID {
		return -1
	}
	if b.ServerID > other.ServerID {
		return 1
	}
	return 0
}

// GreaterThan returns true if this ballot is greater than other
func (b BallotNumber) GreaterThan(other BallotNumber) bool {
	return b.Compare(other) > 0
}

// LessThan returns true if this ballot is less than other
func (b BallotNumber) LessThan(other BallotNumber) bool {
	return b.Compare(other) < 0
}

// Equal returns true if ballots are equal
func (b BallotNumber) Equal(other BallotNumber) bool {
	return b.Compare(other) == 0
}

func (b BallotNumber) String() string {
	return fmt.Sprintf("(%d,%d)", b.TermNumber, b.ServerID)
}

// OperationType defines the type of operation in a log entry
type OperationType string

const (
	OpPut    OperationType = "Put"
	OpDelete OperationType = "Delete"
	OpGet    OperationType = "Get"
	OpNoOp   OperationType = "No-Op"
)

// LogEntry represents an atomic command for the state machine
// LogEntry = (Index, Ballot Number N, Operation Type, Key, Value)
type LogEntry struct {
	Index        int           // Sequential integer defining global ordering
	BallotNum    BallotNumber  // Leader's stable N for safety check
	OpType       OperationType // Command type (Put, Delete, Get, No-Op)
	Key          string        // Key for the operation
	Value        string        // Value for the operation
}

// NewLogEntry creates a new log entry
func NewLogEntry(index int, ballotNum BallotNumber, opType OperationType, key, value string) LogEntry {
	return LogEntry{
		Index:     index,
		BallotNum: ballotNum,
		OpType:    opType,
		Key:       key,
		Value:     value,
	}
}

func (l LogEntry) String() string {
	return fmt.Sprintf("LogEntry[idx=%d, ballot=%s, op=%s, key=%s, val=%s]",
		l.Index, l.BallotNum.String(), l.OpType, l.Key, l.Value)
}

// KVStore represents the in-memory key-value store (state machine)
type KVStore struct {
	store map[string]string
}

// NewKVStore creates a new KV store
func NewKVStore() *KVStore {
	return &KVStore{
		store: make(map[string]string),
	}
}

// Put stores a key-value pair
func (kv *KVStore) Put(key, value string) {
	kv.store[key] = value
}

// Get retrieves a value by key
func (kv *KVStore) Get(key string) (string, bool) {
	val, exists := kv.store[key]
	return val, exists
}

// Delete removes a key from the store
func (kv *KVStore) Delete(key string) {
	delete(kv.store, key)
}

// Apply applies a log entry to the KV store state machine
func (kv *KVStore) Apply(entry LogEntry) string {
	switch entry.OpType {
	case OpPut:
		kv.Put(entry.Key, entry.Value)
		return entry.Value
	case OpDelete:
		kv.Delete(entry.Key)
		return ""
	case OpGet:
		val, _ := kv.Get(entry.Key)
		return val
	case OpNoOp:
		return ""
	}
	return ""
}

// Size returns the number of entries in the store
func (kv *KVStore) Size() int {
	return len(kv.store)
}

type ServerState int

const (
	Follower ServerState = iota
	Candidate
	Leader
	Default
)

func (s ServerState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}