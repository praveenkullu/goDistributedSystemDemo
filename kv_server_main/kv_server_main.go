package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"goDistributedSystemDemo/kv_server_main/kvserver"
)

func main() {
	serverAddr := flag.String("addr", "localhost:8001", "KV server address (host:port)")
	vsAddr := flag.String("vs", "localhost:8000", "View service address (host:port)")
	flag.Parse()

	fmt.Printf("Starting KV Server on %s\n", *serverAddr)
	fmt.Printf("View Service at %s\n", *vsAddr)

	kv := kvserver.StartServer(*serverAddr, *vsAddr)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down KV Server...")
	kv.Kill()
}
