package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

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
	log.Println("Received /status request. Fetching config and polling PLCs.")

	// Fetch the config *each time* status is requested.
	configData, err := FetchConfigurationFromAPI(app.Config)
	if err != nil {
		log.Printf("Error fetching config for status: %v", err)
		http.Error(w, "Failed to fetch config for status", http.StatusInternalServerError)
		return
	}

	status, err := ReadStatusFromPLCs(app.Config, configData) // Pass configData
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
