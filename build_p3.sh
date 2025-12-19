#!/bin/bash

# Multi-Paxos KV Store Build Script

set -e

echo "Building Multi-Paxos Key-Value Store..."

# Generate protobuf files
echo "Generating protobuf files..."
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    multipaxos_kvstore/proto/paxos.proto

# Build server
echo "Building server..."
go build -o bin/paxos_server multipaxos_kvstore/main.go

# Build client
echo "Building client..."
go build -o bin/paxos_client multipaxos_kvstore/client/client.go

echo "Build complete!"
echo "Binaries are in ./bin/"
echo ""
echo "To start a 5-node cluster, run:"
echo " ./bin/paxos_server -id 0 -port 8001 -init_role Leader -peers 5"
echo " ./bin/paxos_server -id 1 -port 8002 -peers 5" 
echo " ./bin/paxos_server -id 2 -port 8003 -peers 5"
echo " ./bin/paxos_server -id 3 -port 8004 -peers 5"
echo " ./bin/paxos_server -id 4 -port 8005 -peers 5"
echo ""
echo "To start the client:"
echo "  ./bin/paxos_client"
