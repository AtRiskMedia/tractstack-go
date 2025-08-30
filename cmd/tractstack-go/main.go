package main

import (
	"log"
	"os"

	"github.com/AtRiskMedia/tractstack-go/internal/application/startup"
)

func main() {
	if err := startup.Initialize(); err != nil {
		log.Fatalf("Application startup failed: %v", err)
		os.Exit(1)
	}

	log.Println("Application has shut down gracefully.")
}
