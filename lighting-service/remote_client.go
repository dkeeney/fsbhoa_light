package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
        "net/url"
        "strconv"
        "strings"
	"time"
)

// PollResponse matches the JSON returned by the Bluehost PHP script
type PollResponse struct {
	Status string `json:"status"`
	JobID  int    `json:"job_id"`
	ZoneID int    `json:"zone_id"`
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
			log.Printf("Job #%d Received: Turn on Zone %s for %s", job.JobID, job.ZoneID, job.Email)

			// 3. Verify Swipe (Local WP API)
                        if verifyLocalSwipe(app.Config, job.Email) {
                            // Get the duration from your config (default to 90 mins)
                            duration := app.Config.QRCodeActuatedDuration
                            if duration == 0 { duration = 90 }

                            allowed, reason := isQRRequestAllowed(app.Config, app.PLCConfig, job.ZoneID, duration)
                            if !allowed {
                                log.Printf("Job #%d REJECTED: %s", job.JobID, reason)
            
                                // Report specific status back so the UI can show "Outside Window"
                                app.reportStatus(job.JobID, reason) 
                                continue // Skip activation and wait for the next job
                            }

                            // Calculate the future "Off Time"
                            offTime := time.Now().Add(time.Duration(duration) * time.Minute)

                            // Convert to HHMM format (e.g., 17:30 -> 1730)
                            qroffValue := uint16(offTime.Hour()*100 + offTime.Minute())

                            // Use the ZoneID directly in your activation function
                            if app.PLCConfig != nil {
                                if err := SetZoneQROff(app.Config, app.PLCConfig, job.ZoneID, qroffValue); err != nil {
				    log.Printf("Job #%d Error: PLC write failed: %v", job.JobID, err)
				    app.reportStatus(job.JobID, "error_plc_failed")
			        } else {
				    log.Printf("Job #%d Success: Zone %d activated.", job.JobID, job.ZoneID)
				    app.reportStatus(job.JobID, "success")
                                }
                            }
                        } else {
                            log.Printf("Job #%d: Swipe not verified.", job.JobID)
                            app.updateJobStatusOnBluehost(job.JobID, "denied_no_swipe")
                        }
		}

		time.Sleep(1 * time.Second)
	}
}

func (app *App) updateJobStatusOnBluehost(jobID int, newStatus string) {
    // You'll need an endpoint on Bluehost like 'update_job.php'
    // or use your existing API structure.
    apiURL := fmt.Sprintf("%s/update_job.php?job_id=%d&status=%s", 
              app.Config.BluehostURL, jobID, newStatus)
    
    req, _ := http.NewRequest("POST", apiURL, nil)
    req.Header.Set("X-API-Key", app.Config.BluehostAPIKey)
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err == nil {
        resp.Body.Close()
    }
}

// Helper: Performs the actual HTTP request to Bluehost
func performLongPoll(client *http.Client, cfg Config) (*PollResponse, error) {
        // Note: Ensure cfg.BluehostURL includes the base path to wait_for_job
        url := fmt.Sprintf("%swait-for-job", cfg.BluehostURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// AUTH & FORMAT HEADERS
        req.Header.Set("User-Agent", "curl/7.81.0")
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
        url := fmt.Sprintf("%s/wp-json/fsbhoa/v1/access/verify-email?email=%s", cfg.AccessControlURL, url.QueryEscape(email))

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-KEY", cfg.AccessControlAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Swipe Verification Error: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}


func (app *App) reportStatus(jobID int, status string) {
	// Note: Using /update_job to match your Bluehost folder structure
	apiURL := fmt.Sprintf("%supdate-job?job_id=%d&status=%s",
		app.Config.BluehostURL, jobID, status)

	req, _ := http.NewRequest("POST", apiURL, nil)
	req.Header.Set("X-API-Key", app.Config.BluehostAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to report status to Bluehost: %v", err)
		return
	}
	defer resp.Body.Close()
}

// Check if at least part of the QR request duration will fall within a QR Enabled window.
func isQRRequestAllowed(cfg Config, configData *FullConfigurationData, zoneID int, durationMinutes int) (bool, string) {
	// 1. Find the Zone and Schedule
	var targetZone *FullConfigZone
	for _, z := range configData.Zones {
		if z.ID == zoneID {
			targetZone = &z
			break
		}
	}
	if targetZone == nil { return false, "zone_not_found" }

	var targetSched *FullConfigSchedule
	for _, s := range configData.Schedules {
		if s.ID == targetZone.ScheduleID {
			targetSched = &s
			break
		}
	}
	if targetSched == nil { return false, "schedule_not_found" }

        // 2. Current Time Context
	now := time.Now()
	today := strings.ToLower(now.Format("Mon"))
	currentPLC := uint16(now.Hour()*100 + now.Minute())

	endTime := now.Add(time.Duration(durationMinutes) * time.Minute)
	endPLC := uint16(endTime.Hour()*100 + endTime.Minute())
        lat := cfg.Latitude
        lon := cfg.Longitude
        // Safety Fallback: If config is missing them, default to Lodge coords
        if lat == 0 && lon == 0 {
                lat = 35.3733
                lon = -119.0187
        }

	// --- DYNAMIC SUNSET / SUNRISE (Bakersfield, CA) ---
	// Coordinates: Latitude ~35.37, Longitude ~-119.01
	riseUTC, setUTC := sunrise.SunriseSunset(lat, lon, now.Year(), now.Month(), now.Day())
	
	// Convert UTC to local server time
	rise := riseUTC.In(time.Local)
	set := setUTC.In(time.Local)

	// Define an early acceptance window (e.g., allow scan 60 mins before schedule start)
	earlyBuffer := 60 * time.Minute

	// Calculate dynamic Photocell bounds with the early buffer applied
	bufferedSet := set.Add(-earlyBuffer)
	PhotoStart := uint16(bufferedSet.Hour()*100 + bufferedSet.Minute())
	PhotoEnd := uint16(rise.Hour()*100 + rise.Minute())

	log.Printf("DEBUG SOLAR: Using Lat: %f, Lon: %f", lat, lon)
        log.Printf("DEBUG SOLAR: Actual Sunrise: %s | Actual Sunset: %s", rise.Format("15:04"), set.Format("15:04"))
        log.Printf("DEBUG SOLAR: Early Buffer Applied: %s", bufferedSet.Format("15:04"))
        log.Printf("DEBUG SOLAR: PLC Integer Targets -> PhotoStart: %d | PhotoEnd: %d", PhotoStart, PhotoEnd)

	// 3. Check Spans
	qrSpanDefined := false // Track if we find ANY QR-enabled spans
	for _, span := range targetSched.Spans {
		trig := strings.ToUpper(span.OnTrigger)
		isQRType := (trig == "QR_PHOTOCELL" || trig == "QR_SUNDOWN" || trig == "QR_TIME")
		if !isQRType {
			continue
		}

		qrSpanDefined = true
		dayMatch := false
		for _, d := range span.DaysOfWeek {
			if strings.ToLower(d) == today {
				dayMatch = true
				break
			}
		}
		if !dayMatch { continue }

		var sStart, sEnd uint16

		if trig == "QR_PHOTOCELL" || trig == "QR_SUNDOWN" {
			sStart = PhotoStart
			sEnd = PhotoEnd
		} else if trig == "QR_TIME" {
			if span.OnTime == nil || span.OffTime == nil { continue }
			
			sVal, _ := strconv.Atoi(strings.ReplaceAll(*span.OnTime, ":", ""))
			eVal, _ := strconv.Atoi(strings.ReplaceAll(*span.OffTime, ":", ""))
			
			// Safely subtract the buffer using base-60 time math
			hours := sVal / 100
			mins := sVal % 100
			startTimeObj := time.Date(now.Year(), now.Month(), now.Day(), hours, mins, 0, 0, time.Local)
			bufferedStart := startTimeObj.Add(-earlyBuffer)
			
			sStart = uint16(bufferedStart.Hour()*100 + bufferedStart.Minute())
			sEnd = uint16(eVal)
		} else {
			continue
		}

		// --- OVERLAP CHECK (Handling Midnight Cross) ---
		isMidnightSpan := sEnd < sStart

		if isMidnightSpan {
			if currentPLC >= sStart || currentPLC <= sEnd || endPLC >= sStart || endPLC <= sEnd {
				return true, ""
			}
		} else {
			if (currentPLC >= sStart && currentPLC <= sEnd) || (endPLC >= sStart && endPLC <= sEnd) {
				return true, ""
			}
		}
	}

	if !qrSpanDefined {
		return false, "qr_not_defined"
	}

	return false, "outside_qr_window"}



