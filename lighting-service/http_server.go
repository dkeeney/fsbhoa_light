package main

import (
	"encoding/json"
        "fmt"
	"log"
	"net/http"
	"strconv"
        "sync"
        "time"

	"github.com/julienschmidt/httprouter"
)

// App holds our application state, like the config.
type App struct {
	Config Config
        simulatedState      map[string]bool
	simulatedStateMutex sync.RWMutex
}

// isSimulationMode checks if PLC addresses are configured. If not, we're in sim mode.
func (app *App) isSimulationMode() bool {
	// Check if a PLC is configured. If not, we're in simulation mode.
	if plc1Addr, ok := app.Config.PLCs[1]; !ok || plc1Addr == "" {
		return true
	}
	return false
}

// RunServer starts the main HTTP server.
func (app *App) RunServer() error {
	router := httprouter.New()
	// Renamed handler to clarify it just *triggers* the sync now
	router.POST("/sync", app.handleSyncTrigger)
	router.POST("/override/zone/:id/:state", app.handleOverride)
	router.GET("/status", app.handleStatus)
        router.POST("/test/mapping/:id/:state", app.handleTestMapping)

	// Use ListenPort from config
	return http.ListenAndServe(app.Config.ListenPort, router)
}

// handleSyncTrigger is triggered by WordPress when config changes.
// It will fetch the *latest* config from WP and push it.
func (app *App) handleSyncTrigger(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("Received /sync trigger. Fetching latest config from WordPress API and pushing to PLCs.")

	// Fetch the full configuration from WordPress API
	configData, err := FetchConfigurationFromAPI(app.Config) // NEW function call
	if err != nil {
		log.Printf("Error fetching config from API: %v", err)
		http.Error(w, "Failed to fetch config from WordPress", http.StatusInternalServerError)
		return
	}

	// Translate the config into PLC data and push it.
        if !app.isSimulationMode() {
		log.Println("Pushing config to PLCs...")
		// Translate the config into PLC data and push it.
		err = PushConfigurationToPLCs(app.Config, configData) // Existing function call
		if err != nil {
			log.Printf("Error pushing config to PLCs: %v", err)
			http.Error(w, "Failed to push config to PLCs", http.StatusInternalServerError)
			return
		}
	} else {
		log.Println("Simulation mode: Skipping PLC config push.")
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

        if app.isSimulationMode() {
		err = app.setSimulatedState(configData, zoneID, state)
	} else {
		err = PulseZone(app.Config, configData, zoneID, state) // Pass configData
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleStatus needs the config to know which outputs/inputs to read.
func (app *App) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("Received /status request. Fetching config and polling PLCs.")

        var status map[string]interface{}
	var err error

	// --- Check for simulation mode ---
	if app.isSimulationMode() {
		log.Println("Simulation mode: Reading from in-memory state.")
		status, err = app.getSimulatedState()
	} else {
		log.Println("Live mode: Fetching config and polling PLCs.")
		// Fetch the config *each time* status is requested.
		configData, err := FetchConfigurationFromAPI(app.Config)
		if err != nil {
			log.Printf("Error fetching config for status: %v", err)
			http.Error(w, "Failed to fetch config for status", http.StatusInternalServerError)
			return
		}
		status, err = ReadStatusFromPLCs(app.Config, configData) // Pass configData
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}


// getSimulatedState reads from the internal map in a thread-safe way.
func (app *App) getSimulatedState() (map[string]interface{}, error) {
	app.simulatedStateMutex.RLock()         // Lock for reading
	defer app.simulatedStateMutex.RUnlock() // Unlock when done

	// Create a new map to return
	status := make(map[string]interface{})

	// Copy all the values
	for key, value := range app.simulatedState {
		status[key] = value
	}

	// Manually add Photocell so the UI doesn't break
	if _, ok := status["Photocell"]; !ok {
		status["Photocell"] = false // Simulate daylight
	}

	return status, nil
}

// setSimulatedState writes to the internal map in a thread-safe way.
func (app *App) setSimulatedState(configData *FullConfigurationData, zoneID int, state string) error {
	// This logic mimics how the UI determines state: by the *first* output in the mapping.
	for _, mapping := range configData.Mappings {
		isForThisZone := false
		for _, linkedZoneID := range mapping.LinkedZoneIDs {
			if linkedZoneID == zoneID {
				isForThisZone = true
				break
			}
		}

		if isForThisZone && len(mapping.PLCOutputs) > 0 {
			outputToToggle := mapping.PLCOutputs[0] // Get the "ON" output
                        plcID := mapping.PLCID
			
                        uniqueKey := fmt.Sprintf("PLC%d-%s", plcID, outputToToggle) // e.g., "PLC1-Y101"
			app.simulatedStateMutex.Lock() // Lock for writing
			if state == "on" {
				app.simulatedState[uniqueKey] = true
			} else {
				app.simulatedState[uniqueKey] = false
			}
			log.Printf("SIMULATOR: Set %s = %t", outputToToggle, app.simulatedState[uniqueKey])
			app.simulatedStateMutex.Unlock() // Unlock
		}
	}
	return nil
}


// startTimeSyncer runs a continuous loop to keep PLC clocks in sync.
func (app *App) startTimeSyncer() {
	if app.isSimulationMode() {
		log.Println("Simulation mode: Skipping background time sync.")
		return
	}

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
		if err := SetPLCTime(host); err != nil {
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

	if app.isSimulationMode() {
		log.Println("Simulation: Test Pulse ignored.")
	} else {
		err = PulseMapping(app.Config, configData, mappingID, state)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
