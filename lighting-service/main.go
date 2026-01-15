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
	ListenPort          string         `json:"ListenPort"`
        LogFilePath         string         `json:"LogFilePath"`
	PLCs       map[int]string          `json:"PLCs"`
        WordPressAPIKey     string         `json:"WordPressAPIKey"`
	WordPressAPIBaseURL string         `json:"WordPressAPIBaseURL"`
        QRCodeActuatedDuration int         `json:"QRCodeActuatedDuration"` // Default duration in minutes
	BluehostURL         string         `json:"BluehostURL"`            // Remote Polling URL
	BluehostAPIKey      string         `json:"BluehostAPIKey"`         // Remote API Key
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

        // Start the Remote Poller (Bluehost or Internal Test)
	if cfg.BluehostURL != "" && cfg.BluehostAPIKey != "" {
		go startBluehostPoller(cfg)
	} else {
		log.Println("Remote integration disabled (URL or API Key missing).")
	}

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





type PollResponse struct {
	Status string `json:"status"`
	JobID  int    `json:"job_id"`
	Court  string `json:"court"`
	Email  string `json:"email"`
}

func startBluehostPoller(cfg Config) {
	log.Printf("Started Remote Poller connecting to: %s", cfg.BluehostURL)

	// Custom Client with longer timeout than the PHP script (70s vs 60s)
	client := &http.Client{ Timeout: 70 * time.Second }

	for {
		// A. Long Poll Request
		job, err := performLongPoll(client, cfg)
		
		if err != nil {
			log.Printf("Poller Error (Retrying in 10s): %v", err)
			time.Sleep(10 * time.Second) // Backoff
			continue
		}

		// B. Handle Response
		if job.Status == "timeout" {
			continue // No work, loop immediately
		}

		if job.Status == "found" {
			log.Printf("Job #%d Received: Turn on %s for %s", job.JobID, job.Court, job.Email)

			// C. Verify Swipe (Local WP API)
			if verifyLocalSwipe(cfg, job.Email) {
				log.Printf("Job #%d: Swipe Verified. Activating...", job.JobID)
				
				// Fetch full config to map "tennis" -> Zone ID
				fullConfig, err := FetchConfigurationFromAPI(cfg)
				if err != nil {
					log.Printf("Job #%d Error: Could not fetch local config mapping: %v", job.JobID, err)
					continue
				}

				// Find Zone ID by Name
				zoneID := findZoneIDByName(fullConfig, job.Court)
				if zoneID > 0 {
					// ACTIVATE!
					err := PulseZone(cfg, fullConfig, zoneID, "on")
					if err == nil {
						// Assuming we just log success for now since PHP handles the stashing
						log.Printf("Job #%d Success: Lights activated.", job.JobID)
					} else {
						log.Printf("Job #%d Error: PLC Pulse failed: %v", job.JobID, err)
					}
				} else {
					log.Printf("Job #%d Error: Court '%s' not found in local configuration.", job.JobID, job.Court)
				}
			} else {
				log.Printf("Job #%d Denied: No valid gate swipe found.", job.JobID)
			}
		}
		
		time.Sleep(1 * time.Second)
	}
}

func performLongPoll(client *http.Client, cfg Config) (*PollResponse, error) {
	req, err := http.NewRequest("GET", cfg.BluehostURL, nil)
	if err != nil { return nil, err }
	
	req.Header.Set("X-API-Key", cfg.BluehostAPIKey)

	resp, err := client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result PollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func verifyLocalSwipe(cfg Config, email string) bool {
	// Call Local WP API
	url := fmt.Sprintf("%s/wp-json/fsbhoa-lighting/v1/verify-swipe?email=%s", cfg.WordPressAPIBaseURL, email)
	
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-KEY", cfg.WordPressAPIKey)
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Swipe Verification Error: %v", err)
		return false // Fail safe
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		return true
	}
	return false
}

func findZoneIDByName(data *FullConfigurationData, name string) int {
	for _, z := range data.Zones {
		// Case-insensitive match would be better, but exact match is fine for start
		if strings.EqualFold(z.ZoneName, name) { return z.ID }
	}
	return 0
}
