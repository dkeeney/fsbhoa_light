package main

import (
	"encoding/json"
        "io"
        "io/ioutil" // Use ioutil for simple file reading
	"log"
        "os"
)

// Config struct holds all our settings.
type Config struct {
	ListenPort string         `json:"ListenPort"`
        LogFilePath         string            `json:"LogFilePath"`
	PLCs       map[int]string `json:"PLCs"`
        WordPressAPIKey     string         `json:"WordPressAPIKey"`
	WordPressAPIBaseURL string         `json:"WordPressAPIBaseURL"`
}

const configFilePath = "/var/lib/fsbhoa/lighting_service.json"

func main() {
	log.Println("Starting FSBHOA Lighting Service...")

	// --- Load Configuration from JSON file ---
        cfg := Config{ 
            ListenPort: ":8085", 
            LogFilePath: "~/fsbhoa_light/lighting-service/lighting-service.log",
            PLCs: make(map[int]string),
        }
        configData, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Printf("WARNING: Could not read config file '%s': %v. Using defaults.", configFilePath, err)
	} else {
		err = json.Unmarshal(configData, &cfg)
		if err != nil {
	             log.Printf("WARNING: Could not parse config file '%s': %v. Using defaults.", configFilePath, err)
		     // Reset cfg to defaults if JSON parsing fails to avoid partial config
                     cfg = Config{
                         ListenPort: ":8085", 
                         LogFilePath: "~/fsbhoa_light/lighting-service/lighting-service.log",
                         PLCs: make(map[int]string),
                     }
		}
	}
        // --- CONFIGURE THE LOGGER BASED ON THE CONFIG ---
	// If LogFilePath is "stdout" or empty, log to console (which is the default)
	if cfg.LogFilePath != "" && cfg.LogFilePath != "stdout" {
		logFile, err := os.OpenFile(cfg.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			log.Printf("ERROR: could not open log file '%s': %v", cfg.LogFilePath, err)
			log.Println("Logging to standard output instead.")
		} else {
			// Set the logger to write to both the file and standard output
			mw := io.MultiWriter(os.Stdout, logFile)
			log.SetOutput(mw)
		}
	}

	log.Printf("Loaded configuration: %+v", cfg) // Log the loaded config

	// --- Start the HTTP Server ---
        app := &App{
		Config:         cfg,
		simulatedState: make(map[string]bool), // Initialize the state map
		// The mutex is fine with its zero-value
	}

        // Since the PLC has nstp service, we no longer need to force the time.
        //go app.startTimeSyncer()

	log.Printf("Starting HTTP server on %s...", cfg.ListenPort)
	if err := app.RunServer(); err != nil { // Use ListenPort from config
		log.Fatalf("Could not start server: %v", err)
	}
}
