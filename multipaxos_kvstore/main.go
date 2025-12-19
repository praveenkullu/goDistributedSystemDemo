package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	// "multipaxos_kvstore/server"
	"goDistributedSystemDemo/multipaxos_kvstore/server"
	// "goDistributedSystemDemo/multipaxos_kvstore/types"
)

func main() {
	// Command line flags
	serverID := flag.Int("id", 0, "Server ID (0-4 for a 5-node cluster)")
	port := flag.Int("port", 8001, "Port to listen on")
	// init_role := flag.String("init_role", "Follower", "role of the server")
	init_role := flag.Bool("role_leader", false, "Initial role of the server: true for Leader, false for Follower")
	N_peers := flag.Int("peers", 5, "number of peers in the cluster")
	dataDir := flag.String("data", "./bin/data", "Data directory for persistent storage")
	flag.Parse()

	// Validate server ID
	if *serverID < 0 || *serverID > 4 {
		log.Fatal("Server ID must be between 0 and 4 for a 5-node cluster")
	}

	// Set default data directory if not specified
	if *dataDir == "" {
		*dataDir = fmt.Sprintf("../data/server_%d", *serverID)
	}

	// Create data directory
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Define all peer addresses (N=5 cluster)
	// Each server knows about all other servers
	fmt.Println("Number of peers in the cluster:", *N_peers)
	var peers []string
	for idx:=1; idx<=*N_peers; idx++ {
		peers = append(peers, fmt.Sprintf("localhost:%d", 8000+idx))
	}
	// peers := []string{
		// "localhost:8001", // Server 0
		// "localhost:8002", // Server 1
		// "localhost:8003", // Server 2
		// "localhost:8004", // Server 3
		// "localhost:8005", // Server 4
	// }

	log.Printf("Starting Multi-Paxos Server %d", *serverID)
	log.Printf("Port: %d", *port)
	log.Printf("Data directory: %s", *dataDir)
	log.Printf("Peers: %v", peers)

	// Create persistent state directory
	stateDir := filepath.Join(*dataDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Fatalf("Failed to create state directory: %v", err)
	}
	
	// var init_state types.ServerState

	// switch *init_role {
	// case "Follower":
	// 	init_state = types.Follower
	// case "Candidate":
	// 	init_state = types.Candidate
	// case "Leader":
	// 	init_state = types.Leader
	// default:
	// 	log.Fatalf("Invalid init_role: %s. Must be 'Follower', 'Candidate', or 'Leader'.", *init_role)
	// }

	
	// init_state := types.ServerState(0)
	// init_state := func() types.ServerState {
	// 	if *init_role == 0 {
	// 		return types.Follower
	// 	} else if *init_role == 1 {
	// 		return types.Candidate
	// 	} else {
	// 		return types.Leader
	// 	}
	// }

	// Create Paxos server
	paxosServer, err := server.NewPaxosServer(*serverID, peers, stateDir, *init_role)
	if err != nil {
		log.Fatalf("Failed to create Paxos server: %v", err)
	}

	// Create gRPC server
	grpcServer, err := server.NewGRPCServer(paxosServer, *port)
	if err != nil {
		log.Fatalf("Failed to create gRPC server: %v", err)
	}

	// Start Paxos server event loops
	paxosServer.Start()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down server...")
		grpcServer.Stop()
		paxosServer.Stop()
		os.Exit(0)
	}()

	// Start gRPC server (blocking)
	log.Printf("Server %d ready and listening on port %d", *serverID, *port)
	if err := grpcServer.Start(); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
