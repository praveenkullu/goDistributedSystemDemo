package persistence

import (
	"fmt"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"goDistributedSystemDemo/multipaxos_kvstore/types"
)

// PersistentState, persisted datastructure to disk
type PersistentState struct {
	mu sync.RWMutex

	// Log: Array of LogEntry objects
	Log []types.LogEntry

	// BallotNumber (Npromised): (Term Number, Server ID)
	Npromised types.BallotNumber

	// HighestAcceptedBallot: (Term Number, Server ID) per Log Index
	HighestAcceptedBallot map[int]types.BallotNumber

	// File path for persistence
	filePath string
}

// persistentData is the structure that gets serialized to disk
type persistentData struct {
	Log                   []types.LogEntry            `json:"log"`
	Npromised             types.BallotNumber          `json:"npromised"`
	HighestAcceptedBallot map[int]types.BallotNumber `json:"highest_accepted_ballot"`
}

// NewPersistentState creates a new persistent state manager
func NewPersistentState(serverID int, dataDir string) (*PersistentState, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dataDir, fmt.Sprintf("persistent_state_id_%d.json", serverID))

	ps := &PersistentState{
		Log:                   make([]types.LogEntry, 0),
		Npromised:             types.NewBallotNumber(0, serverID),
		HighestAcceptedBallot: make(map[int]types.BallotNumber),
		filePath:              filePath,
	}

	// Try to load existing state from disk
	if err := ps.load(); err != nil {
		// If file doesn't exist, that's okay - we'll start fresh
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return ps, nil
}

// AppendLog appends a new log entry and persists to disk
// This must be called after accepting and appending a new LogEntry
func (ps *PersistentState) AppendLog(entry types.LogEntry) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Append to log
	ps.Log = append(ps.Log, entry)

	// Persist to disk before returning
	return ps.save()
}

// UpdateLog updates a log entry at a specific index
func (ps *PersistentState) UpdateLog(index int, entry types.LogEntry) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if index >= 0 && index < len(ps.Log) {
		ps.Log[index] = entry
	} else {
		// Extend log if necessary
		for len(ps.Log) <= index {
			ps.Log = append(ps.Log, types.LogEntry{})
		}
		ps.Log[index] = entry
	}

	return ps.save()
}

// SetNpromised updates the promised ballot number and persists to disk
// Must be called before sending a Promise response (Phase 1)
func (ps *PersistentState) SetNpromised(ballot types.BallotNumber) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.Npromised = ballot

	// Persist to disk before returning
	return ps.save()
}

// SetHighestAcceptedBallot updates the highest accepted ballot for a log index
// Must be called after successfully accepting a LogEntry (Phase 2)
func (ps *PersistentState) SetHighestAcceptedBallot(index int, ballot types.BallotNumber) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.HighestAcceptedBallot[index] = ballot

	// Persist to disk before returning
	return ps.save()
}

// GetNpromised returns the current promised ballot number
func (ps *PersistentState) GetNpromised() types.BallotNumber {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.Npromised
}

// GetLog returns a copy of the log
func (ps *PersistentState) GetLog() []types.LogEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	logCopy := make([]types.LogEntry, len(ps.Log))
	copy(logCopy, ps.Log)
	return logCopy
}

// GetLogEntry returns a specific log entry
func (ps *PersistentState) GetLogEntry(index int) (types.LogEntry, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if index >= 0 && index < len(ps.Log) {
		return ps.Log[index], true
	}
	return types.LogEntry{}, false
}

// GetLogLength returns the length of the log
func (ps *PersistentState) GetLogLength() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.Log)
}

// GetHighestAcceptedBallot returns the highest accepted ballot for a log index
func (ps *PersistentState) GetHighestAcceptedBallot(index int) (types.BallotNumber, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	ballot, exists := ps.HighestAcceptedBallot[index]
	return ballot, exists
}

// GetAllAcceptedEntries returns all log entries that have been accepted
func (ps *PersistentState) GetAllAcceptedEntries() []types.LogEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	entries := make([]types.LogEntry, 0)
	for i, entry := range ps.Log {
		if _, accepted := ps.HighestAcceptedBallot[i]; accepted {
			entries = append(entries, entry)
		}
	}
	return entries
}

// save persists the current state to disk (must be called with lock held)
func (ps *PersistentState) save() error {
	data := persistentData{
		Log:                   ps.Log,
		Npromised:             ps.Npromised,
		HighestAcceptedBallot: ps.HighestAcceptedBallot,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first
	tmpPath := ps.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpPath, ps.filePath)
}

// load reads the persisted state from disk
func (ps *PersistentState) load() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Read file
	jsonData, err := os.ReadFile(ps.filePath)
	if err != nil {
		return err
	}

	// Unmarshal JSON
	var data persistentData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	// Restore state
	ps.Log = data.Log
	ps.Npromised = data.Npromised
	ps.HighestAcceptedBallot = data.HighestAcceptedBallot

	return nil
}
