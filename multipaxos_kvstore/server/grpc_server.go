package server

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"

	pb "goDistributedSystemDemo/multipaxos_kvstore/proto"
	"goDistributedSystemDemo/multipaxos_kvstore/types"
)

// GRPCServer wraps the PaxosServer and implements gRPC service interfaces
type GRPCServer struct {
	pb.UnimplementedPaxosServiceServer
	pb.UnimplementedClientServiceServer

	paxosServer *PaxosServer
	grpcServer  *grpc.Server
	listener    net.Listener
}

// NewGRPCServer creates a new gRPC server wrapper
func NewGRPCServer(paxosServer *PaxosServer, port int) (*GRPCServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	server := &GRPCServer{
		paxosServer: paxosServer,
		grpcServer:  grpcServer,
		listener:    listener,
	}

	// Register services
	pb.RegisterPaxosServiceServer(grpcServer, server)
	pb.RegisterClientServiceServer(grpcServer, server)

	return server, nil
}

// Start begins serving gRPC requests
func (s *GRPCServer) Start() error {
	log.Printf("gRPC server listening on %s", s.listener.Addr().String())
	return s.grpcServer.Serve(s.listener)
}

// Stop gracefully stops the gRPC server
func (s *GRPCServer) Stop() {
	s.grpcServer.GracefulStop()
}

// ============================================================================
// PaxosService implementation
// ============================================================================

// Prepare handles Phase 1: Prepare RPC
func (s *GRPCServer) Prepare(ctx context.Context, req *pb.PrepareRequest) (*pb.PrepareResponse, error) {
	if req.Ballot == nil {
		return nil, fmt.Errorf("ballot is required")
	}

	ballot := types.NewBallotNumber(int(req.Ballot.TermNumber), int(req.Ballot.ServerId))

	promise, responseBallot, acceptedEntries, err := s.paxosServer.HandlePrepare(ctx, ballot)
	if err != nil {
		return nil, err
	}

	// Convert accepted entries to protobuf
	pbEntries := make([]*pb.LogEntry, 0, len(acceptedEntries))
	for _, entry := range acceptedEntries {
		pbEntries = append(pbEntries, convertLogEntryToPB(entry))
	}

	return &pb.PrepareResponse{
		Promise: promise,
		Ballot: &pb.BallotNumber{
			TermNumber: int32(responseBallot.TermNumber),
			ServerId:   int32(responseBallot.ServerID),
		},
		AcceptedEntries: pbEntries,
	}, nil
}

// Accept handles Phase 2: Accept RPC
func (s *GRPCServer) Accept(ctx context.Context, req *pb.AcceptRequest) (*pb.AcceptResponse, error) {
	if req.Ballot == nil {
		return nil, fmt.Errorf("ballot is required")
	}
	if req.Entry == nil {
		return nil, fmt.Errorf("entry is required")
	}

	ballot := types.NewBallotNumber(int(req.Ballot.TermNumber), int(req.Ballot.ServerId))
	entry := convertPBToLogEntry(req.Entry)

	accepted, index, responseBallot, err := s.paxosServer.HandleAccept(ctx, ballot, entry)
	if err != nil {
		return nil, err
	}

	return &pb.AcceptResponse{
		Accepted: accepted,
		Index:    int32(index),
		Ballot: &pb.BallotNumber{
			TermNumber: int32(responseBallot.TermNumber),
			ServerId:   int32(responseBallot.ServerID),
		},
	}, nil
}

// Heartbeat handles heartbeat from leader
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if req.LeaderBallot == nil {
		return nil, fmt.Errorf("leader ballot is required")
	}

	ballot := types.NewBallotNumber(int(req.LeaderBallot.TermNumber), int(req.LeaderBallot.ServerId))
	// success := s.paxosServer.HandleHeartbeat(ballot, int(req.CommitIndex))

	// return &pb.HeartbeatResponse{
	// 	Success: success,
	// }, nil
	success, LastLogEntryIndex := s.paxosServer.HandleHeartbeat(ballot, int(req.CommitIndex))

	return &pb.HeartbeatResponse{
		Success:           success,
		LastLogEntryIndex: int32(LastLogEntryIndex),
	}, nil
}

// TransferState handles state transfer from leader
func (s *GRPCServer) TransferState(ctx context.Context, req *pb.StateTransfer) (*pb.StateTransferResult, error) {
	if req.MissingEntries == nil {
		return &pb.StateTransferResult{
			Success: true,
		}, nil
	}

	// Convert missing entries from protobuf
	missingEntries := make([]types.LogEntry, 0, len(req.MissingEntries))
	for _, pbEntry := range req.MissingEntries {
		entry := convertPBToLogEntry(pbEntry)
		missingEntries = append(missingEntries, entry)
	}

	// Handle state transfer
	success, err := s.paxosServer.HandleStateTransfer(ctx, missingEntries)
	if err != nil {
		return nil, err
	}

	return &pb.StateTransferResult{
		Success: success,
	}, nil
}

// ============================================================================
// ClientService implementation
// ============================================================================

// Put handles client Put operation
func (s *GRPCServer) Put(ctx context.Context, req *pb.ClientPutRequest) (*pb.ClientResponse, error) {
	success, message, err := s.paxosServer.Put(req.Key, req.Value)

	if err != nil {
		return &pb.ClientResponse{
			Success: success,
			Message: message,
		}, nil
	}

	return &pb.ClientResponse{
		Success: true,
		Message: "OK",
	}, nil
}

// Get handles client Get operation
func (s *GRPCServer) Get(ctx context.Context, req *pb.ClientGetRequest) (*pb.ClientResponse, error) {
	success, value, err := s.paxosServer.Get(req.Key)

	if err != nil {
		return &pb.ClientResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.ClientResponse{
		Success: success,
		Value:   value,
		Message: "OK",
	}, nil
}

// Delete handles client Delete operation
func (s *GRPCServer) Delete(ctx context.Context, req *pb.ClientDeleteRequest) (*pb.ClientResponse, error) {
	success, message, err := s.paxosServer.Delete(req.Key)

	if err != nil {
		return &pb.ClientResponse{
			Success: success,
			Message: message,
		}, nil
	}

	return &pb.ClientResponse{
		Success: true,
		Message: "OK",
	}, nil
}
