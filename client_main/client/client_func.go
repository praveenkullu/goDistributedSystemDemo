package client

import (
	"context"
	"log"
	"time"

	pb "goDistributedSystemDemo/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a client for the KV service
type Client struct {
	vsAddress      string
	vsClient       pb.ViewServiceClient
	vsConn         *grpc.ClientConn
	currentPrimary string
	primaryClient  pb.KVServerClient
	primaryConn    *grpc.ClientConn
}

// MakeClient creates a new client
func MakeClient(vsAddress string) *Client {
	ck := &Client{
		vsAddress:      vsAddress,
		currentPrimary: "",
	}

	// Connect to view service
	for {
		conn, err := grpc.Dial(vsAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			ck.vsConn = conn
			ck.vsClient = pb.NewViewServiceClient(conn)
			log.Printf("Client connected to view service at %s\n", vsAddress)
			break
		}
		log.Printf("Failed to connect to view service, retrying...\n")
		time.Sleep(1 * time.Second)
	}

	return ck
}

// Get retrieves the value for a key
func (ck *Client) Get(key string) string {
	req := &pb.GetRequest{Key: key}

	for {
		// Get current primary
		if ck.currentPrimary == "" {
			ck.updatePrimary()
			if ck.currentPrimary == "" {
				time.Sleep(500 * time.Millisecond)
				continue
			}
		}

		// Try to call Get on primary
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := ck.primaryClient.Get(ctx, req)
		cancel()

		if err == nil && resp.Ok {
			return resp.Value
		} else if err == nil && resp.Error == "ErrNoKey" {
			return ""
		} else if err != nil || resp.Error == "ErrNotPrimary" {
			// Primary changed or failed, update and retry
			log.Printf("Get failed, updating primary and retrying...\n")
			ck.currentPrimary = ""
			if ck.primaryConn != nil {
				ck.primaryConn.Close()
				ck.primaryConn = nil
				ck.primaryClient = nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Put stores a key-value pair
func (ck *Client) Put(key string, value string) {
	req := &pb.PutRequest{Key: key, Value: value}

	for {
		// Get current primary
		if ck.currentPrimary == "" {
			ck.updatePrimary()
			if ck.currentPrimary == "" {
				time.Sleep(500 * time.Millisecond)
				continue
			}
		}

		// Try to call Put on primary
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := ck.primaryClient.Put(ctx, req)
		cancel()

		if err == nil && resp.Ok {
			return
		} else if err != nil || resp.Error == "ErrNotPrimary" {
			// Primary changed or failed, update and retry
			log.Printf("Put failed, updating primary and retrying...\n")
			ck.currentPrimary = ""
			if ck.primaryConn != nil {
				ck.primaryConn.Close()
				ck.primaryConn = nil
				ck.primaryClient = nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// updatePrimary queries the view service for the current primary
func (ck *Client) updatePrimary() {
	req := &pb.GetViewRequest{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	resp, err := ck.vsClient.GetView(ctx, req)
	cancel()

	if err != nil {
		log.Printf("GetView failed: %v\n", err)
		return
	}

	if resp.View.Primary != "" && resp.View.Primary != ck.currentPrimary {
		ck.currentPrimary = resp.View.Primary
		if ck.primaryConn != nil {
			ck.primaryConn.Close()
		}

		// Connect to new primary
		conn, err := grpc.Dial(ck.currentPrimary, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Failed to connect to primary %s: %v\n", ck.currentPrimary, err)
			ck.currentPrimary = ""
			return
		}
		ck.primaryConn = conn
		ck.primaryClient = pb.NewKVServerClient(conn)
		log.Printf("Client connected to primary %s\n", ck.currentPrimary)
	}
}

// Close closes the client connections
func (ck *Client) Close() {
	if ck.vsConn != nil {
		ck.vsConn.Close()
	}
	if ck.primaryConn != nil {
		ck.primaryConn.Close()
	}
}
