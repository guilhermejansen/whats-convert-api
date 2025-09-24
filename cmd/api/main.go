package main

import (
	"log"
	"os"

	"whats-convert-api/internal/config"
	"whats-convert-api/internal/server"
)

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[MediaConverter] ")

	// Load configuration
	cfg := config.Load()

	// Create server
	srv := server.New(cfg)

	// Initialize server
	if err := srv.Initialize(); err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
		os.Exit(1)
	}

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
		os.Exit(1)
	}
}