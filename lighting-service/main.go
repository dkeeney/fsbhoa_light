package main

import (
	"encoding/json"
        "io/ioutil" // Use ioutil for simple file reading
	"log"
)

// Config struct holds all our settings.
type Config struct {
	ListenPort string         `json:"ListenPort"`
	PLCs       map[int]string `json:"PLCs"`
        WordPressAPIKey     string         `json:"WordPressAPIKey"`
	WordPressAPIBaseURL string         `json:"WordPressAPIBaseURL"`
}

const configFilePath = "/var/lib/fsbhoa/lighting_service.json"

func main() {
	log.Println("Starting FSBHOA Lighting Service...")

	// --- Load Configuration from JSON file ---
        cfg := Config{ ListenPort: ":8085", PLCs: make(map[int]string) } // Defaults
        configData, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Printf("WARNING: Could not read config file '%s': %v. Using defaults.", configFilePath, err)
	} else {
		err = json.Unmarshal(configData, &cfg)
		if err != nil {
	             log.Printf("WARNING: Could not parse config file '%s': %v. Using defaults.", configFilePath, err)
		     // Reset cfg to defaults if JSON parsing fails to avoid partial config
                     cfg = Config{ListenPort: ":8085", PLCs: make(map[int]string)}
		}
	}
	log.Printf("Loaded configuration: %+v", cfg) // Log the loaded config

	// --- Start the HTTP Server ---
	app := &App{ Config: cfg }
	log.Printf("Starting HTTP server on %s...", cfg.ListenPort)
	if err := app.RunServer(); err != nil { // Use ListenPort from config
		log.Fatalf("Could not start server: %v", err)
	}
}
