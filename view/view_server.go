package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"goDistributedSystemDemo/view/viewservice"
)

func main() {
	address := flag.String("addr", "localhost:8000", "View service address (host:port)")
	flag.Parse()

	fmt.Printf("Starting View Service on %s\n", *address)
	pid := os.Getpid()
	fmt.Printf("PID: %d\n", pid)
	vs := viewservice.StartServer(*address)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down View Service...")
	vs.Kill()
}
