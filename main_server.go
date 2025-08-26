package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Parse command line flags
	port := flag.String("port", "8080", "Port to run the server on")
	flag.Parse()

	// Create and start server
	server := NewServer(*port)

	// Handle graceful shutdown
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c

		log.Println("\n🛑 Shutting down server...")
		server.Cleanup()
		os.Exit(0)
	}()

	// Start server
	log.Fatal(server.Start())
}
