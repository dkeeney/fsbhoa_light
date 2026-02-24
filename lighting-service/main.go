package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

// Config struct
type Config struct {
	ListenPort             string         `json:"ListenPort"`
	LogFilePath            string         `json:"LogFilePath"`
	PLCs                   map[int]string `json:"PLCs"`
	LightingAPIKey         string         `json:"LightingAPIKey"`
	LightingAPIBaseURL     string         `json:"LightingAPIBaseURL"`
	BluehostURL            string         `json:"BluehostURL"`
	BluehostAPIKey         string         `json:"BluehostAPIKey"`
        AccessControlAPIKey    string         `json:"AccessControlAPIKey"`
        AccessControlURL       string         `json:"AccessControlURL"`
	QRCodeActuatedDuration int            `json:"QRCodeActuatedDuration"`
        Latitude               float64        `json:"Latitude"`
        Longitude              float64        `json:"Longitude"`
        IgnoreSolarCheck       bool           `json:"IgnoreSolarCheck"`
}


// Global config variable
var globalConfig Config

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	var cfg Config
	err = json.NewDecoder(file).Decode(&cfg)
	return cfg, err
}

func main() {
	log.Println("Starting FSBHOA Lighting Service...")

	// 1. Load Configuration
	configPath := "/var/lib/fsbhoa/lighting_service.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	globalConfig = cfg

	// 2. Setup Logging
	var logOutput io.Writer = os.Stdout
	if cfg.LogFilePath != "" && cfg.LogFilePath != "stdout" {
		logFile, err := os.OpenFile(cfg.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Error opening log file: %v", err)
		}
		logOutput = io.MultiWriter(os.Stdout, logFile)
	}
	log.SetOutput(logOutput)
	log.Printf("Loaded configuration: %+v", cfg)

	// 3. Initialize App
	app := &App{
		Config:         cfg,
	}

        // Fetch initial config so the system isn't "blank" on boot
        log.Println("Fetching initial configuration...")
        initialCfg, err := FetchConfigurationFromAPI(app.Config)
        if err == nil {
            app.PLCConfig = initialCfg
        } else {
            log.Printf("Warning: Initial config fetch failed: %v", err)
        }

	// 4. Start Background Services
	
	// A. Remote Client (Bluehost) - Delegated to remote_client.go
	if cfg.BluehostURL != "" && cfg.BluehostAPIKey != "" {
		go app.StartBluehostPoller()
	} else {
		log.Println("Remote integration disabled (URL or API Key missing).")
	}

	// B. Time Syncer (PLC Clocks) - Delegated to http_server.go
	go app.startTimeSyncer()


	// 5. Start HTTP Server - Delegated to http_server.go
	log.Printf("Starting HTTP server on %s...", cfg.ListenPort)
	if err := app.RunServer(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}


