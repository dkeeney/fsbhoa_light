package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/goburrow/modbus"
)

// --- Data Structures  ---
type FullConfigZone struct {
	ID         int    `json:"id"`
	ZoneName   string `json:"zone_name"`
	ScheduleID int    `json:"schedule_id"`
}
type FullConfigMapping struct {
	ID            int      `json:"id"`
	PLCID         int      `json:"plc_id"`
	PLCOutputs    []string `json:"plc_outputs"`
	LinkedZoneIDs []int    `json:"linked_zone_ids"`
}
type FullConfigSchedule struct {
	ID           int              `json:"id"`
	ScheduleName string           `json:"schedule_name"`
	Spans        []FullConfigSpan `json:"spans"`
}
type FullConfigSpan struct {
	DaysOfWeek []string `json:"days_of_week"`
	OnTrigger  string   `json:"on_trigger"`
	OnTime     *string  `json:"on_time"`
	OffTrigger string   `json:"off_trigger"`
	OffTime    *string  `json:"off_time"`
}
type FullConfigurationData struct {
	Zones     []FullConfigZone     `json:"zones"`
	Mappings  []FullConfigMapping  `json:"mappings"`
	Schedules []FullConfigSchedule `json:"schedules"`
}

// --- Main Functions ---

// FetchConfigurationFromAPI 
func FetchConfigurationFromAPI(cfg Config) (*FullConfigurationData, error) {
	// ... (This function is identical to your last version) ...
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
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("wordpress API returned non-200 status: %s - %s", resp.Status, string(bodyBytes))
	}
	var configData FullConfigurationData
	if err := json.NewDecoder(resp.Body).Decode(&configData); err != nil {
		return nil, fmt.Errorf("could not decode API response: %w", err)
	}
	return &configData, nil
}

// --- HELPER: calculateLoopIndex ---
// Duplicates the logic from PulseZone to find the 0-23 index for a light
func calculateLoopIndex(yOutput string) int {
	yNum, _ := strconv.Atoi(yOutput[1:]) // e.g., 101
	if yNum == 0 {
		return -1 // Invalid output
	}
	moduleGroup := (yNum - (yNum % 100)) / 100 // e.g., 1
	outputOnModule := (yNum % 100)           // e.g., 1
	outputPairIndex := (outputOnModule - 1) / 2
	loopIndex := (moduleGroup-1)*8 + outputPairIndex
	
	if loopIndex < 0 || loopIndex > 23 {
		return -1 // Invalid index
	}
	return loopIndex
}

// ---  HELPER: generateScheduleBlock ---
// Creates the 70-register block for a single schedule
func generateScheduleBlock(schedule FullConfigSchedule) []uint16 {
	scheduleBlock := make([]uint16, 70) // 14 spans * 5 regs
	for i, span := range schedule.Spans {
		if i >= 14 {
			break
		} // Max 14 spans
		offset := i * 5
		scheduleBlock[offset+0] = daysToBitmask(span.DaysOfWeek)
		scheduleBlock[offset+1], scheduleBlock[offset+2] = triggerToPLCData(span.OnTrigger, span.OnTime)
		scheduleBlock[offset+3], scheduleBlock[offset+4] = triggerToPLCData(span.OffTrigger, span.OffTime)
	}
	return scheduleBlock
}

// ---  PushConfigurationToPLCs ---
func PushConfigurationToPLCs(cfg Config, data *FullConfigurationData) error {
	log.Println("Starting configuration push to all PLCs...")

	// 1. --- Schedule Remapping ---
	// Create a map of [WordPress_DB_ID] -> [PLC_ID_1_to_12]
	dbID_to_plcID := make(map[int]int)
	// Create a map of [PLC_ID_1_to_12] -> [70-register-data-block]
	plcScheduleBlocks := make(map[int][]uint16)

	for i, schedule := range data.Schedules {
		plcID := i + 1 // 1-based index
		if plcID > 12 {
			log.Printf("Warning: More than 12 schedules in WordPress. Ignoring schedule '%s' (ID %d) and beyond.", schedule.ScheduleName, schedule.ID)
			break
		}
		log.Printf("Mapping DB Sched ID %d (%s) -> PLC Sched Slot %d", schedule.ID, schedule.ScheduleName, plcID)
		dbID_to_plcID[schedule.ID] = plcID
		plcScheduleBlocks[plcID] = generateScheduleBlock(schedule)
	}

	// 2. --- Map Block Generation (DS1000-DS1023) ---
	// Create a map of [Zone_ID] -> [WordPress_DB_ID]
	zone_to_schedDB_ID := make(map[int]int)
	for _, zone := range data.Zones {
		zone_to_schedDB_ID[zone.ID] = zone.ScheduleID
	}

	// Create a map for each PLC's schedule map
	// map[plcID 1 or 2] -> [24-register-array]
	plcMaps := make(map[int][]uint16)
	plcMaps[1] = make([]uint16, 24) // Map for PLC 1
	plcMaps[2] = make([]uint16, 24) // Map for PLC 2

	// Populate the 24-register maps for each PLC
	for _, mapping := range data.Mappings {
		if len(mapping.PLCOutputs) == 0 {
			continue // Skip empty mappings
		}

		loopIndex := calculateLoopIndex(mapping.PLCOutputs[0])
		if loopIndex == -1 {
			log.Printf("Warning: Skipping mapping '%s' with invalid output '%s'", mapping.ID, mapping.PLCOutputs[0])
			continue
		}

		// Find the schedule for this light
		if len(mapping.LinkedZoneIDs) == 0 {
			continue // No zone linked
		}
		zoneID := mapping.LinkedZoneIDs[0]
		schedDB_ID := zone_to_schedDB_ID[zoneID]
		plcSchedID := dbID_to_plcID[schedDB_ID] // This is the new ID (1-12) or 0
		
		if _, ok := plcMaps[mapping.PLCID]; ok {
			plcMaps[mapping.PLCID][loopIndex] = uint16(plcSchedID)
		}
	}

	// 3. --- Write Blocks to PLCs ---
	for plcID, host := range cfg.PLCs {
		log.Printf("Connecting to PLC %d at %s...", plcID, host)
		handler := modbus.NewTCPClientHandler(host)
		handler.Timeout = 10 * time.Second
		client := modbus.NewClient(handler)
		err := handler.Connect()
		if err != nil {
			log.Printf("  - ERROR connecting to PLC %d: %v", plcID, err)
			continue
		}
		defer handler.Close()

		// A. Write all 12 Schedule Blocks
		log.Printf("  - Writing 12 schedule blocks to PLC %d...", plcID)
		for i := 1; i <= 12; i++ {
			startAddress := scheduleIDToModbusAddress(i) // Gets 99, 169, 239...
			blockData, ok := plcScheduleBlocks[i]
			if !ok {
				blockData = make([]uint16, 70) // Send an empty block
			}
			
			_, err := client.WriteMultipleRegisters(startAddress, uint16(len(blockData)), u16SliceToBytes(blockData))
			if err != nil {
				log.Printf("  - ERROR writing schedule slot %d to PLC %d: %v", i, plcID, err)
			}
		}

		// B. Write the 24-register Map Block
		mapBlock, ok := plcMaps[plcID]
		if !ok {
			log.Printf("  - ERROR: No map block found for PLC %d", plcID)
			continue
		}
		
		log.Printf("  - Writing 24-register map block to PLC %d...", plcID)
		const mapStartAddress = 999 // DS1000
		_, err = client.WriteMultipleRegisters(mapStartAddress, uint16(len(mapBlock)), u16SliceToBytes(mapBlock))
		if err != nil {
			log.Printf("  - ERROR writing map block to PLC %d: %v", plcID, err)
		}

		// C. Set the Sync Request Bit (C151)
		log.Println("  - All data written. Requesting PLC re-sync...")
		syncRequestAddr, _ := cBitToModbusAddress(151) // C151
		_, err = client.WriteSingleCoil(syncRequestAddr, 0xFF00)
		if err != nil {
			log.Printf("  - ERROR requesting re-sync (SET C151) on PLC %d: %v", plcID, err)
		}
	}

	log.Println("Configuration push finished.")
	return nil
}


// PulseZone
func PulseZone(cfg Config, configData *FullConfigurationData, zoneID int, state string) error {
	log.Printf("Received override for Zone %d. Finding ALL associated lights...", zoneID)

	// --- Create a list of all lights to pulse ---
	type pulseTarget struct {
		host      string
		loopIndex int
		outputs   []string // For logging
	}
	var targets []pulseTarget

	for _, mapping := range configData.Mappings {
		for _, linkedZoneID := range mapping.LinkedZoneIDs {
			if linkedZoneID == zoneID {
				// Found a match. Get its info.
				host, ok := cfg.PLCs[mapping.PLCID]
				if !ok {
					log.Printf("Warning: Skipping pulse for Zone %d. Mapping %d has invalid PLCID %d.", zoneID, mapping.ID, mapping.PLCID)
					continue // Skip this mapping
				}

				if len(mapping.PLCOutputs) == 0 {
					continue // No outputs defined
				}

				loopIndex := calculateLoopIndex(mapping.PLCOutputs[0])
				if loopIndex == -1 {
					log.Printf("Warning: Skipping pulse for Zone %d. Mapping %d has invalid output %s.", zoneID, mapping.ID, mapping.PLCOutputs[0])
					continue // Skip this mapping
				}

				targets = append(targets, pulseTarget{host: host, loopIndex: loopIndex, outputs: mapping.PLCOutputs})

				// Do NOT break; continue searching for more mappings for this zone
			}
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("no valid, mapped lights found for ZoneID %d", zoneID)
	}

	log.Printf("Zone %d is linked to %d lights. Sending pulses...", zoneID, len(targets))

	var lastErr error

	// --- Iterate and pulse every light ---
	for _, target := range targets {
		onCbitAddr, _ := cBitToModbusAddress(201 + target.loopIndex)
		offCbitAddr, _ := cBitToModbusAddress(251 + target.loopIndex)
		var addrToSet uint16
		var stateStr string

		if state == "on" {
			addrToSet = onCbitAddr
			stateStr = fmt.Sprintf("RequestON (C%d)", 201+target.loopIndex)
		} else {
			addrToSet = offCbitAddr
			stateStr = fmt.Sprintf("RequestOFF (C%d)", 251+target.loopIndex)
		}

		log.Printf("  -> Pulsing %s (%s) on PLC %s (Loop %d)", stateStr, target.outputs[0], target.host, target.loopIndex+1)

		// Send the pulse
		err := setPLCBit(target.host, addrToSet)
		if err != nil {
			log.Printf("  -> ERROR pulsing %s: %v", target.host, err)
			lastErr = err // Store the last error we saw
		}
	}

	return lastErr // Return nil if no errors, or the last error encountered
}


// cBitToModbusAddress
func cBitToModbusAddress(cBit int) (uint16, error) {
    // On CLICK PLC, C Control Relays start at Modbus address 16384 (0x4000).
    const cBitBaseAddress = 16384

    if cBit < 1 {
        return 0, fmt.Errorf("c bit %d is out of supported range", cBit)
    }
    
    // C1 -> 16384, C101 -> 16484
    return uint16(cBitBaseAddress + cBit - 1), nil
}

// ReadStatusFromPLCs
func ReadStatusFromPLCs(cfg Config, configData *FullConfigurationData) (map[string]interface{}, error) {
    log.Println("Reading real-time status from all PLCs.")
    fullStatus := make(map[string]interface{})

    // FIX: Use a string key "PLC_ID-LoopIndex" to avoid collisions between PLCs
    loopIndexToMapKey := make(map[string]string)
    plcLoopIndices := make(map[int][]int)

    for _, mapping := range configData.Mappings {
        if len(mapping.PLCOutputs) == 0 {
            continue
        }
        loopIndex := calculateLoopIndex(mapping.PLCOutputs[0])
        if loopIndex == -1 {
            continue
        }

        yName := mapping.PLCOutputs[0]
        uniqueKey := fmt.Sprintf("PLC%d-%s", mapping.PLCID, yName)
        
        // FIX: Create a composite key for internal lookup
        // Example: "1-0" for PLC 1, Index 0
        lookupID := fmt.Sprintf("%d-%d", mapping.PLCID, loopIndex)
        
        loopIndexToMapKey[lookupID] = uniqueKey
        plcLoopIndices[mapping.PLCID] = append(plcLoopIndices[mapping.PLCID], loopIndex)
    }

    for plcID, host := range cfg.PLCs {
        // ... (Connection logic remains the same) ...
        log.Printf("Polling PLC %d at %s", plcID, host)
        handler := modbus.NewTCPClientHandler(host)
        handler.Timeout = 5 * time.Second
        client := modbus.NewClient(handler)
        err := handler.Connect()
        if err != nil {
            log.Printf("  - ERROR connecting to PLC %d for status read: %v", plcID, err)
            continue
        }
        defer handler.Close()

        // Read the C101-C124 State bits
        // FIX: Ensure we use the Offset Address 16384 here too
        stateBitsAddr, _ := cBitToModbusAddress(101) 
        numStateBits := uint16(24) // 24 lights
        resultBytes, err := client.ReadCoils(stateBitsAddr, numStateBits)
        
        if err != nil {
            log.Printf("  - Error reading state bits (C101-C124) on PLC %d: %v", plcID, err)
        } else {
            for i := 0; i < int(numStateBits); i++ {
                // FIX: Look up using the composite key "PLCID-LoopIndex"
                lookupID := fmt.Sprintf("%d-%d", plcID, i)
                
                uiKey, ok := loopIndexToMapKey[lookupID]
                if !ok {
                    continue // This index isn't mapped for this specific PLC
                }

                byteIndex := i / 8
                bitIndex := uint(i % 8)
                if len(resultBytes) > byteIndex {
                    bitValue := (resultBytes[byteIndex] >> bitIndex) & 1
                    fullStatus[uiKey] = (bitValue == 1)
                }
            }
        }

        // Read Photocell (C154 on PLC 1)
        if plcID == 1 {
            photocellAddr, _ := cBitToModbusAddress(154)
            result, err := client.ReadCoils(photocellAddr, 1)
            if err != nil {
                log.Printf("  - Error reading photocell (C154) on PLC 1: %v", err)
            } else if len(result) > 0 {
                fullStatus["Photocell"] = (result[0] & 1) == 1
            }
        }
    }

    return fullStatus, nil
}


// --- Helper Functions ---

// yOutputToModbusAddress ( used by simulator)
func yOutputToModbusAddress(yOutput string) (uint16, error) {
	yOutput = strings.ToUpper(strings.TrimSpace(yOutput))
	if !strings.HasPrefix(yOutput, "Y") {
		return 0, fmt.Errorf("invalid output format: '%s'", yOutput)
	}
	numStr := strings.TrimPrefix(yOutput, "Y")
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid output number: '%s'", numStr)
	}
	switch {
	case num >= 1 && num <= 100:
		return uint16(num - 1), nil
	case num >= 101 && num <= 177:
		return uint16(8256 + (num - 101)), nil
	case num >= 201 && num <= 277:
		return uint16(8320 + (num - 201)), nil
	case num >= 301 && num <= 377:
		return uint16(8384 + (num - 301)), nil
	}
	return 0, fmt.Errorf("output number %d is out of supported range", num)
}

// scheduleIDToModbusAddress now takes a PLC ID (1-12)
func scheduleIDToModbusAddress(plcID int) uint16 {
	// DS1 starts at Modbus 0. DS100 is Modbus 99.
	// Stride is 70 registers.
	// PLC ID 1 -> DS100 (Modbus 99)
	// PLC ID 2 -> DS170 (Modbus 169)
	if plcID <= 0 || plcID > 12 {
		return 0 // Invalid ID
	}
	return uint16(99 + (plcID-1)*70)
}

// daysToBitmask (Updated to lowercase)
func daysToBitmask(days []string) uint16 {
	var mask uint16 = 0
	dayMap := map[string]uint16{
		"sun": 1,
		"mon": 2,
		"tue": 4,
		"wed": 8,
		"thu": 16,
		"fri": 32,
		"sat": 64,
	}
	for _, day := range days {
		mask |= dayMap[day] // Will use lowercase "mon", "tue", etc.
	}
	return mask
}

func triggerToPLCData(trigger string, t *string) (uint16, uint16) {
	var triggerCode, timeCode uint16
	switch trigger {
	case "TIME":
		triggerCode = 0
	case "SUNDOWN", "SUNRISE":
		triggerCode = 1 // Any non-zero value works
	default:
		triggerCode = 0 // Default to TIME
	}
	if t != nil && len(*t) >= 5 {
		timeStr := strings.Replace(*t, ":", "", -1)
		if len(timeStr) >= 4 {
			timeVal, _ := strconv.Atoi(timeStr[:4])
			timeCode = uint16(timeVal)
		}
	}
	return triggerCode, timeCode
}

func u16SliceToBytes(data []uint16) []byte {
	bytes := make([]byte, len(data)*2)
	for i, v := range data {
		bytes[i*2] = byte(v >> 8)
		bytes[i*2+1] = byte(v)
	}
	return bytes
}

func SetPLCTime(host string) error {
	const startAddress = 19 // 0-based offset for SD20
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("SetPLCTime connect error: %w", err)
	}
	defer handler.Close()

	now := time.Now()
	plcDayOfWeek := uint16(now.Weekday() + 1)
	data := []uint16{
		uint16(now.Year()),
		uint16(now.Month()),
		uint16(now.Day()),
		plcDayOfWeek,
		uint16(now.Hour()),
		uint16(now.Minute()),
		uint16(now.Second()),
	}
	byteData := u16SliceToBytes(data)
	_, err = client.WriteMultipleRegisters(startAddress, uint16(len(data)), byteData)
	if err != nil {
		return fmt.Errorf("failed to write time registers: %w", err)
	}
	log.Printf("Successfully set time on %s to: %v", host, now.Format(time.RFC3339))
	return nil
}

func setPLCBit(host string, address uint16) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("setPLCBit connect error: %w", err)
	}
	defer handler.Close()
	_, err = client.WriteSingleCoil(address, 0xFF00)
	if err != nil {
		return fmt.Errorf("failed to write bit: %w", err)
	}
	return nil
}

