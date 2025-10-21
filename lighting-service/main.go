package main

import (
	"log"
)

// Config struct holds all our settings.
type Config struct {
	DBUser     string
	DBPassword string
	DBName     string
	DBHost     string
	PLCs       map[int]string // Maps PLC ID to its IP address and port
}

func main() {
	log.Println("Starting FSBHOA Lighting Service...")

	// In a real app, you'd load this from a config file (e.g., config.json).
	cfg := Config{
		DBUser:     "wp_user",
		DBPassword: "bakersfield123",
		DBName:     "fsbhoa_db",
		DBHost:     "127.0.0.1:3306",
		PLCs: map[int]string{
			1: "127.0.0.1:502", // Lodge PLC
			2: "127.0.0.1:502", // Pool House PLC
		},
	}

	// Create a new App instance, which holds our state.
	app := &App{
		Config: cfg,
	}

	// Start the HTTP server. This will block and run forever.
	log.Println("Starting HTTP server on port 8085...")
	if err := app.RunServer(); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}
