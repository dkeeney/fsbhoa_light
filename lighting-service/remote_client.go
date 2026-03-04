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

	"github.com/nathan-osman/go-sunrise"
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
			// Fix the typo: ZoneID is an int, use %d
			log.Printf("Job #%d Received: Turn on Zone %d for %s", job.JobID, job.ZoneID, job.Email)

			// A. Pre-fetch Zone and Schedule (Single Source of Truth)
			var targetZone *FullConfigZone
			for _, z := range app.PLCConfig.Zones {
				if z.ID == job.ZoneID {
					targetZone = &z
					break
				}
			}

			if targetZone == nil {
				log.Printf("Job #%d REJECTED: Zone %d not found in config", job.JobID, job.ZoneID)
				app.reportStatus(job.JobID, "zone_not_found")
				continue
			}

			// 1. Verify Swipe (Correctly parsing the isValid boolean)
			isValid, rfid := verifyLocalSwipe(app.Config, job.Email)
			if isValid {

				duration := app.Config.QRCodeActuatedDuration
				if duration == 0 {
					duration = 90
				}

				allowed, reason := isQRRequestAllowed(app.Config,
					app.PLCConfig,
					targetZone.ScheduleID,
					duration)
				if !allowed {
					log.Printf("Job #%d REJECTED: %s", job.JobID, reason)
					app.reportStatus(job.JobID, reason) // e.g., "outside_qr_window"
					continue
				}

				offTime := time.Now().Add(time.Duration(duration) * time.Minute)
				qroffValue := uint16(offTime.Hour()*100 + offTime.Minute())

				if app.PLCConfig != nil {
					if err := SetZoneQROff(app.Config, app.PLCConfig, job.ZoneID, qroffValue); err != nil {
						log.Printf("Job #%d Error: PLC write failed: %v", job.JobID, err)
						app.reportStatus(job.JobID, "error_plc_failed")
					} else {
						log.Printf("Job #%d Success: Zone %d activated until %d.", job.JobID, job.ZoneID, qroffValue)
						app.reportStatus(job.JobID, "success")
						go app.LogQRSuccessToWordPress(*job, targetZone.ZoneName, rfid)
					}
				}
			} else {
				// This is the branch that was causing the infinite spinner
				log.Printf("Job #%d: Swipe not verified for %s.", job.JobID, job.Email)
				app.reportStatus(job.JobID, "denied_no_swipe")
			}
		}
		time.Sleep(1 * time.Second)
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
func verifyLocalSwipe(cfg Config, email string) (bool, string) {
	url := fmt.Sprintf("%s/wp-json/fsbhoa/v1/access/verify-email?email=%s", cfg.AccessControlURL, url.QueryEscape(email))

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-KEY", cfg.AccessControlAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Swipe Verification Connection Error: %v", err)
		return false, ""
	}
	defer resp.Body.Close()

	// 1. Check if the server actually responded
	if resp.StatusCode != 200 {
		return false, ""
	}

	// 2. Parse the JSON body to see if it's ACTUALLY valid
	var data struct {
		IsValid bool   `json:"isValid"`
		RFID    string `json:"rfid_id"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("Error decoding swipe response: %v", err)
		return false, ""
	}

	return data.IsValid, data.RFID
}

func (app *App) reportStatus(jobID int, status string) {
	apiURL := fmt.Sprintf("%supdate-job", app.Config.BluehostURL)

	// 1. Prepare form data for the body instead of the URL
	data := url.Values{}
	data.Set("job_id", strconv.Itoa(jobID))
	data.Set("status", status)

	// 2. Pass the encoded data as a Reader
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	// 3. Set Content-Type so WordPress knows how to parse the body
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-API-Key", app.Config.BluehostAPIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to report status to Bluehost: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Bluehost update-job returned status: %d", resp.StatusCode)
	}
}

// Check if at least part of the QR request duration will fall within a QR Enabled window.
func isQRRequestAllowed(cfg Config, configData *FullConfigurationData, scheduleID int, durationMinutes int) (bool, string) {
	// --- MASTER OVERRIDE ---
	if cfg.IgnoreSolarCheck {
		log.Printf("Bypassing solar and schedule checks (IgnoreSolarCheck is ON)")
		return true, ""
	}

	var targetSched *FullConfigSchedule
	for _, s := range configData.Schedules {
		if s.ID == scheduleID {
			targetSched = &s
			break
		}
	}
	if targetSched == nil {
		return false, "schedule_not_found"
	}

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
		isQRType := (trig == "QR_PHOTOCELL" || trig == "QR_SUNDOWN" || trig == "QR_SUNRISE" || trig == "QR_TIME")
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
		if !dayMatch {
			continue
		}

		var sStart, sEnd uint16

		if trig == "QR_PHOTOCELL" || trig == "QR_SUNDOWN" || trig == "QR_SUNRISE" {
			sStart = PhotoStart
			sEnd = PhotoEnd
		} else if trig == "QR_TIME" {
			if span.OnTime == "" || span.OffTime == "" {
				continue
			}

			// Clean the string first (remove colons)
			cleanOn := strings.ReplaceAll(span.OnTime, ":", "")
			cleanOff := strings.ReplaceAll(span.OffTime, ":", "")

			// Truncate to 4 digits if seconds are present
			if len(cleanOn) >= 4 {
				cleanOn = cleanOn[:4]
			}
			if len(cleanOff) >= 4 {
				cleanOff = cleanOff[:4]
			}

			sVal, _ := strconv.Atoi(cleanOn)
			eVal, _ := strconv.Atoi(cleanOff)
			log.Printf("DEBUG: Sched ID %d | Span OnTrigger %s | Times: %d to %d",
				targetSched.ID, span.OnTrigger, sVal, eVal)

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

	return false, "outside_qr_window"
}

func (app *App) LogQRSuccessToWordPress(job PollResponse, zoneName string, rfid string) {
	// We use the 900001 Virtual Serial Number
	payload := map[string]interface{}{
		"Timestamp":    time.Now().Format("2006-01-02 15:04:05"),
		"SerialNumber": "900001",
		"Door":         job.ZoneID,
		"ZoneName":     zoneName, // Matches the 'friendly_name' in ac_doors
		"CardNumber":   rfid,     // The resident's actual card number
		"Reason":       1,        // Standard swipe
		"Granted":      true,
		"EventMessage": fmt.Sprintf("QR Scan: %s activated", zoneName),
	}

	jsonData, _ := json.Marshal(payload)
	apiURL := fmt.Sprintf("%s/wp-json/fsbhoa/v1/monitor/log-event", app.Config.AccessControlURL)

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", app.Config.AccessControlAPIKey) // Use your existing key logic

	client := &http.Client{Timeout: 5 * time.Second}
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Audit Log Error: %v", err)
			return
		}
		defer resp.Body.Close()
	}()
}
