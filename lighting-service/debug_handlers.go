package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/simonvetter/modbus"
)

// ---------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------

func (app *App) handleDebugRegisters(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	rawResults := app.GetRawPLCData()

	type DebugResponse struct {
		PLCID       int                       `json:"plc_id"`
		Host        string                    `json:"host"`
		LightStatus []bool                    `json:"light_status_c101"`
		LightMap    []uint16                  `json:"light_map_ds1000"`       // Added
		SchedIDMap  []uint16                  `json:"sched_id_map_ds1031"`
		SchedState  []uint16                  `json:"sched_state_ds1051"`     // Added
		Schedules   map[string][]ScheduleSpan `json:"schedules_decoded"`      // Keys are now "01", "02"
	}

	var response []DebugResponse

	for _, p := range rawResults {
		// Use string keys with zero-padding ("01", "02") to force JSON sorting
		decodedScheds := make(map[string][]ScheduleSpan)

		for idStr, rawInts := range p.ScheduleSpans {
			// Convert "1" -> 1 -> "01"
			if id, err := strconv.Atoi(idStr); err == nil {
				key := fmt.Sprintf("%02d", id)
				decodedScheds[key] = parseScheduleArray(rawInts)
			}
		}

		response = append(response, DebugResponse{
			PLCID:       p.PLCID,
			Host:        p.Host,
			LightStatus: p.LightStatus,
			LightMap:    p.LightMap,    // Added
			SchedIDMap:  p.SchedIDMap,
			SchedState:  p.SchedState,  // Added
			Schedules:   decodedScheds,
		})
	}

	// Sort response by PLC ID
	sort.Slice(response, func(i, j int) bool {
		return response[i].PLCID < response[j].PLCID
	})

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		http.Error(w, "Failed to encode data", 500)
	}
}

// ---------------------------------------------------------------------
// Data Gathering Logic
// ---------------------------------------------------------------------

type RawPLCData struct {
	PLCID         int
	Host          string
	LightStatus   []bool
	ScheduleSpans map[string][]uint16
	QRTimers      []uint16
	LightMap      []uint16
	SchedIDMap    []uint16
	SchedState    []uint16
}

func (app *App) GetRawPLCData() []RawPLCData {
	var results []RawPLCData

	for key, host := range app.Config.PLCs {
		
		// Parse ID safely from config key
		plcID, _ := strconv.Atoi(fmt.Sprintf("%v", key))

		client, err := modbus.NewClient(&modbus.ClientConfiguration{
			URL:     "tcp://" + host,
			Timeout: 1 * time.Second,
		})
		if err != nil {
			fmt.Printf("Error creating client for %s: %v\n", host, err)
			continue
		}
		if err := client.Open(); err != nil {
			fmt.Printf("Error opening connection to %s: %v\n", host, err)
			continue
		}

		data := RawPLCData{
			PLCID:         plcID,
			Host:          host,
			ScheduleSpans: make(map[string][]uint16),
		}

		// --- Read Coils ---
		// AddrLightStateBase (16484)
		if coils, err := client.ReadCoils(uint16(AddrLightStateBase), NumLights); err == nil {
			data.LightStatus = coils
		} else {
			fmt.Printf("Error reading coils %s: %v\n", host, err)
		}

		// --- Read Holding Registers ---

		// 1. Schedule Spans (Addr 99)
		regsPerSched := uint16(NumSpans * 5)
		for j := 0; j < NumSchedules; j++ {
			offset := uint16(j) * regsPerSched
			startReg := uint16(AddrSchedConfigBase) + offset 

			spans, err := client.ReadRegisters(startReg, regsPerSched, modbus.HOLDING_REGISTER)
			if err == nil {
				data.ScheduleSpans[fmt.Sprintf("%d", j+1)] = spans
			}
		}

		// 2. Schedule ID Map (Addr 1030)
		if regs, err := client.ReadRegisters(uint16(AddrSchedIDMapBase), NumSchedules, modbus.HOLDING_REGISTER); err == nil {
			data.SchedIDMap = regs
		}

		// 3. Light Map (Addr 999) - Added to output
		if regs, err := client.ReadRegisters(uint16(AddrLightSchedMapBase), NumLights, modbus.HOLDING_REGISTER); err == nil {
			data.LightMap = regs
		}

		// 4. Schedule State (Addr 1050) - Added to output
		if regs, err := client.ReadRegisters(uint16(AddrSchedStateBase), NumSchedules, modbus.HOLDING_REGISTER); err == nil {
			data.SchedState = regs
		}

		// 5. QR Timers
		if regs, err := client.ReadRegisters(uint16(AddrQROffTimeBase), 25, modbus.HOLDING_REGISTER); err == nil {
			data.QRTimers = regs
		}

		client.Close()
		results = append(results, data)
	}

	return results
}

// ---------------------------------------------------------------------
// Formatting Helpers
// ---------------------------------------------------------------------

type ScheduleSpan struct {
	Days      string `json:"days"`
	StartTrig uint16 `json:"start_trig"`
	StartTime uint16 `json:"start_time"`
	StopTrig  uint16 `json:"stop_trig"`
	StopTime  uint16 `json:"stop_time"`
}

func parseScheduleArray(raw []uint16) []ScheduleSpan {
	var spans []ScheduleSpan
	for i := 0; i < len(raw)-4; i += 5 {
		if raw[i] == 0 {
			continue
		}
		spans = append(spans, ScheduleSpan{
			Days:      decodeDays(raw[i]),
			StartTrig: raw[i+1],
			StartTime: raw[i+2],
			StopTrig:  raw[i+3],
			StopTime:  raw[i+4],
		})
	}
	return spans
}

func decodeDays(mask uint16) string {
	if mask == 0 {
		return "Never"
	}
	if mask == 127 {
		return "Every Day"
	}
	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	var activeDays []string
	for i := 0; i < 7; i++ {
		if (mask>>i)&1 == 1 {
			activeDays = append(activeDays, dayNames[i])
		}
	}
	return strings.Join(activeDays, ",")
}

