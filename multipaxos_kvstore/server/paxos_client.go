package server

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "goDistributedSystemDemo/multipaxos_kvstore/proto"
	"goDistributedSystemDemo/multipaxos_kvstore/types"
)

// PaxosClient provides RPC client for communication with peers
type PaxosClient struct {
	conn   *grpc.ClientConn
	client pb.PaxosServiceClient
}

// NewPaxosClient creates a new RPC client for a peer
func NewPaxosClient(addr string) (*PaxosClient, error) {
	// Set up connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	client := pb.NewPaxosServiceClient(conn)

	return &PaxosClient{
		conn:   conn,
		client: client,
	}, nil
}

// Close closes the client connection
func (c *PaxosClient) Close() error {
	return c.conn.Close()
}

// Prepare sends a Prepare RPC (Phase 1)
func (c *PaxosClient) Prepare(ctx context.Context, ballot types.BallotNumber) (bool, types.BallotNumber, []types.LogEntry, error) {
	req := &pb.PrepareRequest{
		Ballot: &pb.BallotNumber{
			TermNumber: int32(ballot.TermNumber),
			ServerId:   int32(ballot.ServerID),
		},
	}

	// Set timeout for RPC
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := c.client.Prepare(ctx, req)
	if err != nil {
		return false, types.BallotNumber{}, nil, err
	}

	// Convert response
	responseBallot := types.BallotNumber{}
	if resp.Ballot != nil {
		responseBallot = types.NewBallotNumber(int(resp.Ballot.TermNumber), int(resp.Ballot.ServerId))
	}

	// Convert accepted entries
	acceptedEntries := make([]types.LogEntry, 0, len(resp.AcceptedEntries))
	for _, pbEntry := range resp.AcceptedEntries {
		entry := convertPBToLogEntry(pbEntry)
		acceptedEntries = append(acceptedEntries, entry)
	}

	return resp.Promise, responseBallot, acceptedEntries, nil
}

// Accept sends an Accept RPC (Phase 2)
func (c *PaxosClient) Accept(ctx context.Context, ballot types.BallotNumber, entry types.LogEntry) (bool, int, types.BallotNumber, error) {
	req := &pb.AcceptRequest{
		Ballot: &pb.BallotNumber{
			TermNumber: int32(ballot.TermNumber),
			ServerId:   int32(ballot.ServerID),
		},
		Entry: convertLogEntryToPB(entry),
	}

	// Set timeout for RPC
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := c.client.Accept(ctx, req)
	if err != nil {
		return false, 0, types.BallotNumber{}, err
	}

	// Convert response
	responseBallot := types.BallotNumber{}
	if resp.Ballot != nil {
		responseBallot = types.NewBallotNumber(int(resp.Ballot.TermNumber), int(resp.Ballot.ServerId))
	}

	return resp.Accepted, int(resp.Index), responseBallot, nil
}

// Heartbeat sends a heartbeat to maintain leadership
// func (c *PaxosClient) Heartbeat(ctx context.Context, ballot types.BallotNumber, commitIndex int) (bool, error) {
func (c *PaxosClient) Heartbeat(ctx context.Context, ballot types.BallotNumber, commitIndex int) (bool, int, error) {
	req := &pb.HeartbeatRequest{
		LeaderBallot: &pb.BallotNumber{
			TermNumber: int32(ballot.TermNumber),
			ServerId:   int32(ballot.ServerID),
		},
		CommitIndex: int32(commitIndex),
	}

	// Set timeout for RPC
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	resp, err := c.client.Heartbeat(ctx, req)
	if err != nil {
		return false, -1, err
	}

	return resp.Success, int(resp.LastLogEntryIndex), nil
}

// TransferState sends missing log entries to a follower
func (c *PaxosClient) TransferState(ctx context.Context, missingEntries []types.LogEntry) (bool, error) {
	// Convert missing entries to protobuf
	pbEntries := make([]*pb.LogEntry, 0, len(missingEntries))
	for _, entry := range missingEntries {
		pbEntries = append(pbEntries, convertLogEntryToPB(entry))
	}

	req := &pb.StateTransfer{
		MissingEntries: pbEntries,
	}

	// Set timeout for RPC (longer timeout for state transfer)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.TransferState(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Success, nil
}

// Helper functions to convert between protobuf and internal types

func convertLogEntryToPB(entry types.LogEntry) *pb.LogEntry {
	return &pb.LogEntry{
		Index: int32(entry.Index),
		BallotNum: &pb.BallotNumber{
			TermNumber: int32(entry.BallotNum.TermNumber),
			ServerId:   int32(entry.BallotNum.ServerID),
		},
		OpType: string(entry.OpType),
		Key:    entry.Key,
		Value:  entry.Value,
	}
}

func convertPBToLogEntry(pbEntry *pb.LogEntry) types.LogEntry {
	ballotNum := types.BallotNumber{}
	if pbEntry.BallotNum != nil {
		ballotNum = types.NewBallotNumber(int(pbEntry.BallotNum.TermNumber), int(pbEntry.BallotNum.ServerId))
	}

	return types.LogEntry{
		Index:     int(pbEntry.Index),
		BallotNum: ballotNum,
		OpType:    types.OperationType(pbEntry.OpType),
		Key:       pbEntry.Key,
		Value:     pbEntry.Value,
	}
}
