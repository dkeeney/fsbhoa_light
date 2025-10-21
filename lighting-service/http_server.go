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
	router.POST("/sync", app.handleSync)
	router.POST("/override/zone/:id/:state", app.handleOverride)
	router.GET("/status", app.handleStatus)

	return http.ListenAndServe(":8085", router)
}

// handleSync is triggered by WordPress when a configuration changes.
func (app *App) handleSync(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("Received /sync request. Fetching config and pushing to PLCs.")

	configData, err := FetchConfiguration(app.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Translate the config into PLC data and push it.
	err = PushConfigurationToPLCs(app.Config, configData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Sync successful."))
}

// handleOverride is triggered by the UI for a manual override.
func (app *App) handleOverride(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	zoneID, _ := strconv.Atoi(ps.ByName("id"))
	state := ps.ByName("state") // "on" or "off"
	log.Printf("Received override request for Zone %d to state %s", zoneID, state)

	err := PulseZone(app.Config, zoneID, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleStatus is triggered by the monitor page.
func (app *App) handleStatus(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("Received /status request. Polling PLCs.")
	
	// This function will need to get the full config to know what to read.
	status, err := ReadStatusFromPLCs(app.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

