package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// PollResponse matches the JSON returned by the Bluehost PHP script
type PollResponse struct {
	Status string `json:"status"`
	JobID  int    `json:"job_id"`
	Court  string `json:"court"`
	Email  string `json:"email"`
}

// StartBluehostPoller is the long-running loop that checks the remote server.
func (app *App) StartBluehostPoller() {
	log.Printf("Started Remote Poller connecting to: %s", app.Config.BluehostURL)
	
	// Custom Client with longer timeout than the PHP script (70s vs 60s)
	client := &http.Client{Timeout: 70 * time.Second}

	for {
		// 1. Long Poll Request
		job, err := performLongPoll(client, app.Config)
		if err != nil {
			log.Printf("Poller Error (Retrying in 10s): %v", err)
			time.Sleep(10 * time.Second) // Backoff
			continue
		}

		// 2. Handle Response
		if job.Status == "timeout" {
			continue // No work, loop immediately
		}

		if job.Status == "found" {
			log.Printf("Job #%d Received: Turn on %s for %s", job.JobID, job.Court, job.Email)

			// 3. Verify Swipe (Local WP API)
			if verifyLocalSwipe(app.Config, job.Email) {
				log.Printf("Job #%d: Swipe Verified. Activating...", job.JobID)

				// Fetch full config to map "Tennis Court 1" -> Zone ID
				fullConfig, err := FetchConfigurationFromAPI(app.Config)
				if err != nil {
					log.Printf("Job #%d Error: Could not fetch local config mapping: %v", job.JobID, err)
					continue
				}

				// Find Zone ID by Name
				zoneID := findZoneIDByName(fullConfig, job.Court)
				if zoneID > 0 {
					// ACTIVATE!
					var pulseErr error
					if app.isSimulationMode() {
						pulseErr = app.setSimulatedState(fullConfig, zoneID, "on")
					} else {
						pulseErr = PulseZone(app.Config, fullConfig, zoneID, "on")
					}

					if pulseErr == nil {
						log.Printf("Job #%d Success: Lights activated.", job.JobID)
					} else {
						log.Printf("Job #%d Error: PLC Pulse failed: %v", job.JobID, pulseErr)
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

// Helper: Performs the actual HTTP request to Bluehost
func performLongPoll(client *http.Client, cfg Config) (*PollResponse, error) {
	req, err := http.NewRequest("GET", cfg.BluehostURL, nil)
	if err != nil {
		return nil, err
	}

	// AUTH & FORMAT HEADERS
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", cfg.BluehostAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
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

// Helper: Asks Local WordPress if this email swiped the gate recently
func verifyLocalSwipe(cfg Config, email string) bool {
	url := fmt.Sprintf("%s/wp-json/fsbhoa-lighting/v1/verify-swipe?email=%s", cfg.WordPressAPIBaseURL, email)

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-KEY", cfg.WordPressAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Swipe Verification Error: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// Helper: Case-insensitive lookup for Zone Name -> ID
func findZoneIDByName(data *FullConfigurationData, name string) int {
	for _, z := range data.Zones {
		if strings.EqualFold(z.ZoneName, name) {
			return z.ID
		}
	}
	return 0
}

