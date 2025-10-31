package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil" // Added for reading API response body
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/goburrow/modbus"
)

// --- Data Structures (Mirrors WordPress API Response) ---

type FullConfigZone struct {
	ID         int    `json:"id"`
	ZoneName   string `json:"zone_name"`
	ScheduleID int    `json:"schedule_id"`
}

type FullConfigMapping struct {
	ID           int      `json:"id"`
	PLCID        int      `json:"plc_id"`
	PLCOutputs   []string `json:"plc_outputs"`
	LinkedZoneIDs []int   `json:"linked_zone_ids"`
}

type FullConfigSchedule struct {
	ID            int               `json:"id"`
	ScheduleName  string            `json:"schedule_name"`
	Spans         []FullConfigSpan `json:"spans"`
}

type FullConfigSpan struct {
	DaysOfWeek []string  `json:"days_of_week"`
	OnTrigger  string    `json:"on_trigger"`
	OnTime     *string   `json:"on_time"` // Use pointer to string
	OffTrigger string    `json:"off_trigger"`
	OffTime    *string   `json:"off_time"` // Use pointer to string
}

type FullConfigurationData struct {
	Zones     []FullConfigZone     `json:"zones"`
	Mappings  []FullConfigMapping  `json:"mappings"`
	Schedules []FullConfigSchedule `json:"schedules"`
}

// --- Main Functions ---

// FetchConfigurationFromAPI gets the full config from the WordPress REST API.
func FetchConfigurationFromAPI(cfg Config) (*FullConfigurationData, error) {
	url := fmt.Sprintf("%s/wp-json/fsbhoa-lighting/v1/full-config", cfg.WordPressAPIBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create API request: %w", err)
	}

	req.Header.Set("X-API-KEY", cfg.WordPressAPIKey)
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not execute API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body) // Read error body
		return nil, fmt.Errorf("wordpress API returned non-200 status: %s - %s", resp.Status, string(bodyBytes))
	}

	var configData FullConfigurationData
	if err := json.NewDecoder(resp.Body).Decode(&configData); err != nil {
		return nil, fmt.Errorf("could not decode API response: %w", err)
	}

	return &configData, nil
}

// PushConfigurationToPLCs translates the API config and writes it to the PLCs.
func PushConfigurationToPLCs(cfg Config, data *FullConfigurationData) error {
	log.Println("Starting configuration push to all PLCs...")
	plcScheduleBlocks := make(map[int]map[int][]uint16) // map[plcID][scheduleID] -> data block

	// Create a quick lookup map for schedules
	scheduleMap := make(map[int]FullConfigSchedule)
	for _, schedule := range data.Schedules {
		scheduleMap[schedule.ID] = schedule
	}

	// Determine which schedules need to be written to which PLC
	for _, zone := range data.Zones {
		if zone.ScheduleID == 0 { continue } // Skip zones with no schedule

		schedule, ok := scheduleMap[zone.ScheduleID]
		if !ok { continue } // Schedule not found

		// Find the PLC for this zone (via its mappings)
		var plcID int = 0
		for _, mapping := range data.Mappings {
			for _, linkedZoneID := range mapping.LinkedZoneIDs {
				if linkedZoneID == zone.ID {
					plcID = mapping.PLCID
					break
				}
			}
			if plcID != 0 { break }
		}
		if plcID == 0 { continue } // No mappings found for this zone

		// Ensure the map for this PLC exists
		if _, ok := plcScheduleBlocks[plcID]; !ok {
			plcScheduleBlocks[plcID] = make(map[int][]uint16)
		}

		// Only generate the block if we haven't already for this schedule
		if _, ok := plcScheduleBlocks[plcID][schedule.ID]; !ok {
			scheduleBlock := make([]uint16, 50) // 10 spans * 5 registers
			for i, span := range schedule.Spans {
				if i >= 10 { break }
				offset := i * 5
				scheduleBlock[offset+0] = daysToBitmask(span.DaysOfWeek)
				scheduleBlock[offset+1], scheduleBlock[offset+2] = triggerToPLCData(span.OnTrigger, span.OnTime)
				scheduleBlock[offset+3], scheduleBlock[offset+4] = triggerToPLCData(span.OffTrigger, span.OffTime)
			}
			plcScheduleBlocks[plcID][schedule.ID] = scheduleBlock
		}
	}

	// Write the prepared blocks to the PLCs
	for plcID, scheduleBlocks := range plcScheduleBlocks {
		plcHost, ok := cfg.PLCs[plcID]; if !ok { continue }
		log.Printf("Connecting to PLC %d at %s to write %d schedule(s)...", plcID, plcHost, len(scheduleBlocks))
		
		handler := modbus.NewTCPClientHandler(plcHost); handler.Timeout = 10 * time.Second
		client := modbus.NewClient(handler)
		// Ensure connection before writing multiple blocks
		err := handler.Connect()
		if err != nil {
			log.Printf("  - ERROR connecting to PLC %d: %v", plcID, err)
			continue // Skip this PLC if connection fails
		}
		defer handler.Close()

		for scheduleID, blockData := range scheduleBlocks {
			startAddress := scheduleIDToModbusAddress(scheduleID)
			log.Printf("  - Writing schedule ID %d (len %d) to start address %d", scheduleID, len(blockData), startAddress)
			_, err := client.WriteMultipleRegisters(startAddress, uint16(len(blockData)), u16SliceToBytes(blockData))
			if err != nil {
				log.Printf("  - ERROR writing schedule %d to PLC %d: %v", scheduleID, plcID, err)
			}
		}
	}
	log.Println("Configuration push finished.")
	return nil
}

// PulseZone finds the correct PLC outputs for a zone and pulses them.
func PulseZone(cfg Config, configData *FullConfigurationData, zoneID int, state string) error {
	var targetZone FullConfigZone
	foundZone := false
	for _, z := range configData.Zones {
		if z.ID == zoneID {
			targetZone = z
			foundZone = true
			break
		}
	}
	if !foundZone { return fmt.Errorf("zone with ID %d not found in fetched config", zoneID) }

	mappingIDs := []int{} // Find mapping IDs linked to this zone
    for _, m := range configData.Mappings {
        for _, linkedZoneID := range m.LinkedZoneIDs {
            if linkedZoneID == zoneID {
                mappingIDs = append(mappingIDs, m.ID)
                break
            }
        }
    }
    if len(mappingIDs) == 0 {
        log.Printf("Warning: No hardware mappings found for Zone ID %d", zoneID)
        return nil // Not necessarily an error, just nothing to pulse
    }

	for _, mappingID := range mappingIDs {
		var mapping FullConfigMapping
        foundMapping := false
        for _, m := range configData.Mappings {
            if m.ID == mappingID {
                mapping = m
                foundMapping = true
                break
            }
        }
		if !foundMapping { continue }

		if len(mapping.PLCOutputs) != 2 {
            log.Printf("Warning: Mapping ID %d for Zone %d has invalid PLC outputs defined: %v", mappingID, zoneID, mapping.PLCOutputs)
            continue
        }

		outputToPulse := mapping.PLCOutputs[0] // Default ON pulse
		if state == "off" { outputToPulse = mapping.PLCOutputs[1] }

		plcHost, ok := cfg.PLCs[mapping.PLCID]; if !ok { continue }
		modbusAddr, err := yOutputToModbusAddress(outputToPulse)
		if err != nil { log.Printf("Warning: %v", err); continue }

		log.Printf("Pulsing output %s (address %d) on PLC %d for zone %d", outputToPulse, modbusAddr, mapping.PLCID, targetZone.ID)
		if err := sendPulse(plcHost, modbusAddr, 250*time.Millisecond); err != nil {
			log.Printf("Error pulsing PLC %d for zone %d: %v", mapping.PLCID, targetZone.ID, err)
            // Continue trying other mappings for the same zone
		}
	}
	return nil
}

// ReadStatusFromPLCs reads all relevant coils and inputs.
func ReadStatusFromPLCs(cfg Config, configData *FullConfigurationData) (map[string]interface{}, error) {
	log.Println("Reading real-time status from all PLCs.")
	fullStatus := make(map[string]interface{})
	
	mappingsByPLC := make(map[int][]FullConfigMapping)
	for _, mapping := range configData.Mappings {
		mappingsByPLC[mapping.PLCID] = append(mappingsByPLC[mapping.PLCID], mapping)
	}

	for plcID, host := range cfg.PLCs {
		log.Printf("Polling PLC %d at %s", plcID, host)
		handler := modbus.NewTCPClientHandler(host); handler.Timeout = 5 * time.Second
		client := modbus.NewClient(handler)
		
		// Ensure connection before reading multiple items
		err := handler.Connect()
		if err != nil {
			log.Printf("  - ERROR connecting to PLC %d for status read: %v", plcID, err)
			continue // Skip this PLC if connection fails
		}
		defer handler.Close()

		outputsToRead := make(map[string]uint16) // outputName -> modbusAddr
        for _, mapping := range mappingsByPLC[plcID] {
             for _, outputName := range mapping.PLCOutputs {
                 addr, err := yOutputToModbusAddress(outputName)
                 if err == nil { outputsToRead[outputName] = addr }
             }
        }
        
        // Read Coils (Y outputs)
		// TODO: Optimize this to read blocks of coils instead of one at a time for better performance
        for outputName, addr := range outputsToRead {
             result, err := client.ReadCoils(addr, 1)
             if err != nil { 
                 log.Printf("  - Error reading coil %s on PLC %d: %v", outputName, plcID, err) 
             } else if len(result) > 0 {
                 fullStatus[outputName] = (result[0] & 1) == 1
             } else {
                 log.Printf("  - Warning: Received empty result reading coil %s on PLC %d", outputName, plcID)
             }
        }

		// Read Photocell (X1 on PLC 1)
		if plcID == 1 {
			photocellAddr := uint16(0) // X1 corresponds to Discrete Input address 0
			result, err := client.ReadDiscreteInputs(photocellAddr, 1)
			if err != nil { 
                log.Printf("  - Error reading photocell (X1) on PLC 1: %v", err) 
            } else if len(result) > 0 {
				fullStatus["Photocell"] = (result[0] & 1) == 1
			} else {
                log.Printf("  - Warning: Received empty result reading photocell (X1) on PLC 1")
            }
		}
	}

	return fullStatus, nil
}

// --- Helper Functions ---

func sendPulse(host string, address uint16, duration time.Duration) error {
	handler := modbus.NewTCPClientHandler(host); handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
    err := handler.Connect()
    if err != nil { return fmt.Errorf("sendPulse connect error: %w", err)}
    defer handler.Close()

	if _, err := client.WriteSingleCoil(address, 0xFF00); err != nil { return fmt.Errorf("failed to write ON: %w", err) }
	time.Sleep(duration)
	if _, err := client.WriteSingleCoil(address, 0x0000); err != nil { return fmt.Errorf("failed to write OFF: %w", err) }
	return nil
}

func yOutputToModbusAddress(yOutput string) (uint16, error) {
	yOutput = strings.ToUpper(strings.TrimSpace(yOutput))
	if !strings.HasPrefix(yOutput, "Y") { return 0, fmt.Errorf("invalid output format: '%s'", yOutput) }
	numStr := strings.TrimPrefix(yOutput, "Y")
	num, err := strconv.Atoi(numStr)
	if err != nil { return 0, fmt.Errorf("invalid output number: '%s'", numStr) }
	// CLICK PLC Modbus Address Mapping for Y outputs (Coils)
	switch {
	case num >= 1 && num <= 100:   return uint16(num - 1), nil         // Y1=0, Y2=1,... Y100=99
	case num >= 101 && num <= 177: return uint16(8256 + (num - 101)), nil // Y101=8256, Y102=8257...
	case num >= 201 && num <= 277: return uint16(8320 + (num - 201)), nil // Y201=8320, Y202=8321...
	case num >= 301 && num <= 377: return uint16(8384 + (num - 301)), nil // Y301=8384, Y302=8385...
	// Add more modules here if needed
	}
	return 0, fmt.Errorf("output number %d is out of supported range", num)
}

func scheduleIDToModbusAddress(id int) uint16 {
	// D registers start at Modbus address 0. D1 is address 0, D2 is 1...
	// We start our schedules at D100, which is Modbus address 99.
	// Schedule 1 (DB ID 1) -> D100 (addr 99). Block size is 50 registers.
	// Schedule 2 (DB ID 2) -> D150 (addr 149).
	if id <= 0 { return 0 } // Invalid ID
	return uint16(99 + (id-1)*50)
}

func daysToBitmask(days []string) uint16 {
	var mask uint16 = 0
	dayMap := map[string]uint16{"Sun": 1, "Mon": 2, "Tue": 4, "Wed": 8, "Thu": 16, "Fri": 32, "Sat": 64}
	for _, day := range days { mask |= dayMap[day] }
	return mask
}

func triggerToPLCData(trigger string, t *string) (uint16, uint16) {
	var triggerCode, timeCode uint16
	if trigger == "SUNDOWN" || trigger == "SUNRISE" { triggerCode = 1 }
	if trigger == "TIME" { triggerCode = 2 }

	// t is now a pointer. Check if it's not nil and points to a valid string.
	if t != nil && len(*t) >= 5 {
		// De-reference the pointer with *t to get the string value
		timeStr := strings.Replace(*t, ":", "", -1) 
		if len(timeStr) >= 4 {
			timeVal, _ := strconv.Atoi(timeStr[:4]) // Take HHMM
			timeCode = uint16(timeVal)
		}
	}
	return triggerCode, timeCode
}

func u16SliceToBytes(data []uint16) []byte {
	bytes := make([]byte, len(data)*2)
	for i, v := range data {
		bytes[i*2] = byte(v >> 8)   // High byte
		bytes[i*2+1] = byte(v)       // Low byte
	}
	return bytes
}
