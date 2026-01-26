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
}



// RunServer starts the main HTTP server.
func (app *App) RunServer() error {
	router := httprouter.New()
	// Renamed handler to clarify it just *triggers* the sync now
	router.POST("/sync", app.handleSyncTrigger)
	router.POST("/override/zone/:id/:state", app.handleOverride)
	router.GET("/status", app.handleStatus)
        router.POST("/test/mapping/:id/:state", app.handleTestMapping)
        router.POST("/trigger/zone/:id", app.handleTriggerTimer)

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

	// Fetch the full configuration from WordPress API
	configData, err := FetchConfigurationFromAPI(app.Config) 
	if err != nil {
		log.Printf("Error fetching config from API: %v", err)
		http.Error(w, "Failed to fetch config from WordPress", http.StatusInternalServerError)
		return
	}

	// Translate the config into PLC data and push it.
	log.Println("Pushing config to PLCs...")
	// Translate the config into PLC data and push it.
	err = PushConfigurationToPLCs(app.Config, configData) // Existing function call
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
	zoneID, _ := strconv.Atoi(ps.ByName("id"))
	state := ps.ByName("state") // "on" or "off"
	log.Printf("Received override request for Zone %d to state %s", zoneID, state)

	// Fetch the config *each time* an override happens to ensure we have the latest mappings.
	configData, err := FetchConfigurationFromAPI(app.Config)
	if err != nil {
		log.Printf("Error fetching config for override: %v", err)
		http.Error(w, "Failed to fetch config for override", http.StatusInternalServerError)
		return
	}

	err = PulseZone(app.Config, configData, zoneID, state) // Pass configData
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleStatus needs the config to know which outputs/inputs to read.
func (app *App) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	//log.Println("Received /status request. Fetching config and polling PLCs.")
        w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

        var status map[string]interface{}
	var err error

	// Fetch the config *each time* status is requested.
	configData, err := FetchConfigurationFromAPI(app.Config)
	if err != nil {
		//log.Printf("Error fetching config for status: %v", err)
		http.Error(w, "Failed to fetch config for status", http.StatusInternalServerError)
		return
	}
	status, err = ReadStatusFromPLCs(app.Config, configData) // Pass configData
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
	
	// Fetch config to ensure we have latest mappings
	configData, err := FetchConfigurationFromAPI(app.Config)
	if err != nil {
		http.Error(w, "Failed to fetch config", http.StatusInternalServerError)
		return
	}

	err = PulseMapping(app.Config, configData, mappingID, state)
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

	// 1. Fetch current config to find the mapping for this zone
	configData, err := FetchConfigurationFromAPI(app.Config)
	if err != nil {
		log.Printf("Error fetching config for timer trigger: %v", err)
		http.Error(w, "Failed to fetch configuration", http.StatusInternalServerError)
		return
	}

	// 2. Execute the trigger
	// We pulse the "ON" bit (C201+) for this zone.
	// Your existing PulseZone function in plc_client.go handles finding 
	// all lights associated with this zone and pulsing their ON bits.
	err = PulseZone(app.Config, configData, zoneID, "on")
	if err != nil {
		log.Printf("Error pulsing PLC for zone %d: %v", zoneID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Timer trigger sent for zone %d", zoneID)
}


