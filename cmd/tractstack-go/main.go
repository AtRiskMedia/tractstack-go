// Package main is the entry point for the tractstack-go application server.
package main

import (
	"log"

	"github.com/AtRiskMedia/tractstack-go/internal/application/startup"
)

func main() {
	if err := startup.Initialize(); err != nil {
		log.Fatalf("Application startup failed: %v", err)
	}
	log.Println("Application has shut down gracefully.")
}
