package main

import (
	"flag"
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "goDistributedSystemDemo/multipaxos_kvstore/proto"
)

// KVClient provides a client interface for the Multi-Paxos KV store
type KVClient struct {
	servers []string
	clients []pb.ClientServiceClient
	conns   []*grpc.ClientConn
}

// NewKVClient creates a new KV client
func NewKVClient(servers []string) (*KVClient, error) {
	client := &KVClient{
		servers: servers,
		clients: make([]pb.ClientServiceClient, len(servers)),
		conns:   make([]*grpc.ClientConn, len(servers)),
	}

	// Connect to all servers
	for i, addr := range servers {
		conn, err := grpc.Dial(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			log.Printf("Warning: Failed to connect to server %s: %v", addr, err)
			continue
		}

		client.conns[i] = conn
		client.clients[i] = pb.NewClientServiceClient(conn)
	}

	return client, nil
}

// Close closes all connections
func (c *KVClient) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			conn.Close()
		}
	}
}

// Put sends a Put request to the cluster
// Will retry on different servers if not the leader
func (c *KVClient) Put(key, value string) error {
	req := &pb.ClientPutRequest{
		Key:   key,
		Value: value,
	}

	// Try all servers until we find the leader
	for i, client := range c.clients {
		if client == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.Put(ctx, req)
		cancel()

		if err != nil {
			log.Printf("Put failed on server %d: %v", i, err)
			continue
		}

		if resp.Success {
			fmt.Printf("leader: [%d] | Put('%s', '%s') -> OK\n", i, key, value)
			return nil
		} else {
			log.Printf("Put failed on server %d: %s", i, resp.Message)
			// If not leader, try next server
			if strings.Contains(resp.Message, "not the leader") {
				continue
			}
		}
	}

	return fmt.Errorf("failed to put key after trying all servers")
}

// Get sends a Get request to any server
func (c *KVClient) Get(key string) (string, error) {
	req := &pb.ClientGetRequest{
		Key: key,
	}

	// Try all servers until we get a successful response
	for i, client := range c.clients {
		if client == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := client.Get(ctx, req)
		cancel()

		if err != nil {
			log.Printf("Get failed on server %d: %v", i, err)
			continue
		}

		if resp.Success {
			fmt.Printf("leader: [%d] | Get('%s') -> '%s'\n", i, key, resp.Value)
			return resp.Value, nil
		}
	}

	return "", fmt.Errorf("key not found")
}

// Delete sends a Delete request to the cluster
func (c *KVClient) Delete(key string) error {
	req := &pb.ClientDeleteRequest{
		Key: key,
	}

	// Try all servers until we find the leader
	for i, client := range c.clients {
		if client == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.Delete(ctx, req)
		cancel()

		if err != nil {
			log.Printf("Delete failed on server %d: %v", i, err)
			continue
		}

		if resp.Success {
			fmt.Printf("leader: [%d] | Delete('%s') -> OK\n", i, key)
			return nil
		} else {
			log.Printf("Delete failed on server %d: %s", i, resp.Message)
			// If not leader, try next server
			if strings.Contains(resp.Message, "not the leader") {
				continue
			}
		}
	}

	return fmt.Errorf("failed to delete key after trying all servers")
}

func main() {
	// Command-line flags for non-interactive mode (test automation)
	serverAddr := flag.String("server", "", "Server address (e.g., localhost:8001)")
	operation := flag.String("op", "", "Operation: put, get, or delete")
	key := flag.String("key", "", "Key for the operation")
	value := flag.String("value", "", "Value for put operation")
	N_peers := flag.Int("peers", 5, "number of peers in the cluster")
	flag.Parse()

	var servers []string

	// If -server flag is provided, still connect to all servers for retry logic
	// but we'll try the specified server first
	if *serverAddr != "" {
		// For testing: connect to all servers in the cluster for auto-retry
		// Parse port from server address and build full server list
		for idx:=1; idx<=*N_peers; idx++ {
			servers = append(servers, fmt.Sprintf("localhost:%d", 8000+idx))
		}
	} else {
		// Multi-server mode (default)
		for idx:=1; idx<=*N_peers; idx++ {
			servers = append(servers, fmt.Sprintf("localhost:%d", 8000+idx))
		}
	}
	// // Default server addresses (can be overridden via args)
	// servers := []string{
	// 	"localhost:8001",
	// 	"localhost:8002",
	// 	"localhost:8003",
	// 	"localhost:8004",
	// 	"localhost:8005",
	// }

	// if len(os.Args) > 1 {
	// 	servers = os.Args[1:]
	// }

	client, err := NewKVClient(servers)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Non-interactive mode (for automated testing)
	if *operation != "" {
		switch *operation {
		case "put":
			if *key == "" || *value == "" {
				fmt.Println("Error: put operation requires -key and -value")
				os.Exit(1)
			}
			if err := client.Put(*key, *value); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return

		case "get":
			if *key == "" {
				fmt.Println("Error: get operation requires -key")
				os.Exit(1)
			}
			val, err := client.Get(*key)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(val)
			return

		case "delete":
			if *key == "" {
				fmt.Println("Error: delete operation requires -key")
				os.Exit(1)
			}
			if err := client.Delete(*key); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return

		default:
			fmt.Printf("Error: unknown operation '%s'\n", *operation)
			os.Exit(1)
		}
	}

	// Interactive mode
	fmt.Printf("Connecting to servers: %v\n", servers)
	fmt.Println("\nMulti-Paxos Key-Value Store Client")
	fmt.Println("===================================")
	fmt.Println("Commands:")
	fmt.Println("  put <key> <value>  - Store a key-value pair")
	fmt.Println("  get <key>          - Retrieve a value by key")
	fmt.Println("  delete <key>       - Delete a key")
	fmt.Println("  exit               - Exit the client")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		command := strings.ToLower(parts[0])

		switch command {
		case "put":
			if len(parts) < 3 {
				fmt.Println("Usage: put <key> <value>")
				continue
			}
			key := parts[1]
			value := strings.Join(parts[2:], " ")
			if err := client.Put(key, value); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "get":
			if len(parts) < 2 {
				fmt.Println("Usage: get <key>")
				continue
			}
			key := parts[1]
			if _, err := client.Get(key); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "delete":
			if len(parts) < 2 {
				fmt.Println("Usage: delete <key>")
				continue
			}
			key := parts[1]
			if err := client.Delete(key); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "exit", "quit":
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Printf("Unknown command: %s\n", command)
		}
	}
}
