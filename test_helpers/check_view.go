package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	pb "goDistributedSystemDemo/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	vsAddr := flag.String("vs", "localhost:8000", "View service address")
	flag.Parse()

	// Connect to the view service
	conn, err := grpc.NewClient(*vsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to view service: %v", err)
	}
	defer conn.Close()

	client := pb.NewViewServiceClient(conn)

	// Get the current view
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetView(ctx, &pb.GetViewRequest{})
	if err != nil {
		log.Fatalf("Failed to get view: %v", err)
	}

	// Print the view in a parseable format
	fmt.Printf("View Number: %d\n", resp.View.ViewNumber)
	if resp.View.Primary != "" {
		fmt.Printf("Primary: %s\n", resp.View.Primary)
	} else {
		fmt.Printf("Primary: <none>\n")
	}

	if resp.View.Backup != "" {
		fmt.Printf("Backup: %s\n", resp.View.Backup)
	} else {
		fmt.Printf("Backup: <none>\n")
	}
}
