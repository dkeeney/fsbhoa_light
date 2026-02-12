package main

import (
	"encoding/json"
        "fmt"
	"log"
	"net/http"
        "strconv"
        "time"

	"github.com/julienschmidt/httprouter"
)

// App holds our application state, like the config.
type App struct {
	Config Config
        PLCConfig *FullConfigurationData
}



// RunServer starts the main HTTP server.
func (app *App) RunServer() error {
	router := httprouter.New()
	// Renamed handler to clarify it just *triggers* the sync now
	router.POST("/sync", app.handleSyncTrigger)
	router.POST("/override/zone/:id/:state", app.handleOverride)
	router.GET("/status", app.handleStatus)
        router.POST("/test/mapping/:id/:state", app.handleTestMapping)
        router.POST("/trigger-timer/zone/:id", app.handleTriggerTimer)

	// Use ListenPort from config
	return http.ListenAndServe(app.Config.ListenPort, router)
}


// handleSyncTrigger is triggered by WordPress when config changes.
// It will fetch the *latest* config from WP and push it.
func (app *App) handleSyncTrigger(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
        if r.Header.Get("X-API-Key") != app.Config.LightingAPIKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	log.Println("Received /sync trigger. Fetching latest config from WordPress API and pushing to PLCs.")

	// 1. Fetch the full configuration from WordPress API
	configData, err := FetchConfigurationFromAPI(app.Config) 
	if err != nil {
		log.Printf("Error fetching config from API: %v", err)
		http.Error(w, "Failed to fetch config from WordPress", http.StatusInternalServerError)
		return
	}

        // 2. Update the shared state!
        app.PLCConfig = configData 

        // 3. Push to PLCs
	// Translate the config into PLC data and push it.
	log.Println("Pushing config to PLCs...")
	// Translate the config into PLC data and push it.
	err = PushConfigurationToPLCs(app.Config, app.PLCConfig) 
	if err != nil {
		log.Printf("Error pushing config to PLCs: %v", err)
		http.Error(w, "Failed to push config to PLCs", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Sync successful."))
}

// handleOverride needs the config to know which outputs to pulse.
func (app *App) handleOverride(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var err error
	zoneID, _ := strconv.Atoi(ps.ByName("id"))
	state := ps.ByName("state") // "on" or "off"
	log.Printf("Received override request for Zone %d to state %s", zoneID, state)

        if app.PLCConfig == nil {
            http.Error(w, "Service initializing, try again in a moment", 503)
            return
        }

	err = PulseZone(app.Config, app.PLCConfig, zoneID, state) // Pass configData
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (app *App) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "*")

    var status map[string]interface{}
    var err error

    // Use the SHARED config we already have in memory
    if app.PLCConfig == nil {
        log.Println("Status request received but PLCConfig is nil. Attempting emergency fetch...")
        app.PLCConfig, err = FetchConfigurationFromAPI(app.Config)
        if err != nil {
            http.Error(w, "Configuration not loaded and API fetch failed", http.StatusInternalServerError)
            return
        }
    }

    // Pass the shared app.PLCConfig to the PLC reader
    status, err = ReadStatusFromPLCs(app.Config, app.PLCConfig) 
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(status)
}


// startTimeSyncer runs a continuous loop to keep PLC clocks in sync.
func (app *App) startTimeSyncer() {

	log.Println("Starting background time sync service (runs every hour)...")
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run once immediately on startup
	app.syncAllPLCsTime()

	// Run on every tick
	for range ticker.C {
		app.syncAllPLCsTime()
	}
}

// syncAllPLCsTime iterates over all configured PLCs and sets their time.
func (app *App) syncAllPLCsTime() {
	log.Println("Running hourly time sync for all PLCs...")
	for plcID, host := range app.Config.PLCs {
		if host == "" {
			continue // Skip unconfigured PLCs
		}
		log.Printf("Syncing time for PLC %d at %s...", plcID, host)
		if err := SetPLCTime(plcID, host); err != nil {
			// Just log the error, don't stop the service
			log.Printf("ERROR: Failed to sync time for PLC %d: %v", plcID, err)
		}
	}
}


func (app *App) handleTestMapping(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	mappingID, _ := strconv.Atoi(ps.ByName("id"))
	state := ps.ByName("state")
        var err error
	
        if app.PLCConfig == nil {
            http.Error(w, "Service initializing, try again in a moment", 503)
            return
        }


	err = PulseMapping(app.Config, app.PLCConfig, mappingID, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleTriggerTimer is called when someone clicks the clock icon in WordPress, or activates a QR code.
func (app *App) handleTriggerTimer(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	zoneID, _ := strconv.Atoi(ps.ByName("id"))
	log.Printf("Received TIMER TRIGGER request for Zone %d", zoneID)
        var err error

        if app.PLCConfig == nil {
            http.Error(w, "Service initializing, try again in a moment", 503)
            return
        }

        // 1. Calculate the 'Off' timestamp (The "Bookkeeping")
        duration := time.Duration(app.Config.QRCodeActuatedDuration) * time.Minute
        offTime := time.Now().Add(duration)
        qroffValue := uint16(offTime.Hour()*100 + offTime.Minute())

        // 2. Write QROff to PLC (The "Paperwork")
        // This allows the status panel to see the countdown
        err = SetZoneQROff(app.Config, app.PLCConfig, zoneID, qroffValue)
        if err != nil {
            log.Printf("ERROR: Failed to set QROff for Zone %d: %v", zoneID, err)
            http.Error(w, "Modbus Write Failed", 500)
            return
        }

        log.Printf("SUCCESS: Zone %d QROff set to %s", zoneID, offTime.Format("15:04:05"))
            
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "Timer started for zone %d. Lights will expire at %s", 
                zoneID, offTime.Format("15:04"))
}


