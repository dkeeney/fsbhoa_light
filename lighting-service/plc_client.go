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
        "github.com/nathan-osman/go-sunrise"
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
	OnTime     string   `json:"on_time"`
	OffTrigger string   `json:"off_trigger"`
	OffTime    string   `json:"off_time"`
}
type FullConfigurationData struct {
	Zones     []FullConfigZone     `json:"zones"`
	Mappings  []FullConfigMapping  `json:"mappings"`
	Schedules []FullConfigSchedule `json:"schedules"`
}
type PLCDebugData struct {
	PLCID           int                 `json:"plc_id"`
	Host            string              `json:"host"`
	LightStatus     []bool              `json:"light_status_c101_c124"`
	ScheduleSpans   map[int][]uint16    `json:"schedule_spans_ds100_ds939"` // Key is Slot 1-12
	QRTimers        []uint16            `json:"qr_timers_ds941_ds965"`
	LightMap        []uint16            `json:"light_map_ds1000_ds1023"`
	SchedIDMap      []uint16            `json:"sched_id_map_ds1031_ds1042"`
	SchedState      []uint16            `json:"sched_state_ds1051_ds1062"`
	Error           string              `json:"error,omitempty"`
}


// Memory map in the PLC
//   C101 to C124   -- Current state of the lights
//   C151           -- Sync request.
//   C154           -- Photocell state.
//   C201 to C224   -- RequestON for a light.  Also clear corresponding QREnd
//   C251 to C274   -- RequestOFF for a light. Also clear corresponding QREnd
//   DS100 to DS939     - Location of Schedule configuration
//             12 schedules, each containing 14 spans
//              Span offsets:
//              +0  DayOfWeek - bitmask for days that span is active (0 means span disabled)
//              +1  OnTrig -- turn on by (0 = time, 1 = photocell, 2 QR & time, 3 QR & photocell)
//              +2  OnTime -- Time span starts (HHMM)
//              +3  OffTrig  -- turn off by PhotoCell or Time
//              +4  OffTime -- Time span ends (HHMM) 24hr inclusive
//   DS941 to DS965    - current QROff Time for timed lights (HHMM)  index is light index
//   DS1000 to DS1023    Mapping of Schedule to use for each light.
//   DS1031 to DS1042  - Mapping of database schedule ID for each schedule slot in PLC.
//   DS1051 to DS1062  - State of Schedule (0 = Off, 1 = ON, 2 = QR enabled.



const (
    NumSchedules  = 12   // max schedules 
    NumSpans      = 14   // max spans per schedule
    NumLights     = 24   // max lights per PLC

    cBitBaseAddress = 16384   // base modbus address for coils

    // --- Coils (Discrete) ---
    AddrLightStateBase = cBitBaseAddress + (101 - 1) // C101-C124: Current Status
    AddrSyncReq        = cBitBaseAddress + (151 - 1) // C151: Sync Request
    AddrPhotocell      = cBitBaseAddress + (154 - 1) // C154: Photocell State
    AddrRequestOnBase  = cBitBaseAddress + (201 - 1) // C201-C224: Manual ON
    AddrRequestOffBase = cBitBaseAddress + (251 - 1) // C251-C274: Manual OFF

    // --- Holding Registers (Integers) ---
    AddrTotalSpans = 99 - 1 // DS99  the total number of spans in all schedules.
    
    // Schedule Config: 12 schedules * 14 spans * 5 words per span
    // DS100 to DS939
    AddrSchedConfigBase = 100 - 1     // Schedule   DS100 to DS939
    AddrQROffTimeBase   = 941 - 1     // QR Timers: DS941 to DS965
    AddrLightSchedMapBase = 1000 - 1  // Light-to-Schedule Mapping: DS1000 to DS1023
    AddrSchedIDMapBase    = 1031 - 1  // Schedule ID map.
    AddrSchedStateBase = 1051 - 1 // Schedule Runtime State: DS1051 to DS1062 (0=Off, 1=On, 2=QR Enabled)
)

// --- Main Functions ---

// FetchConfigurationFromAPI 
func FetchConfigurationFromAPI(cfg Config) (*FullConfigurationData, error) {
	url := fmt.Sprintf("%s/wp-json/fsbhoa-lighting/v1/full-config", cfg.LightingAPIBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create API request: %w", err)
	}
        req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-KEY", cfg.LightingAPIKey)
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
	
	if loopIndex < 0 || loopIndex > NumLights-1 {
		return -1 // Invalid index
	}
	return loopIndex
}

// ---  HELPER: generateScheduleBlock ---
// Creates the 70-register block for a single schedule
func generateScheduleBlock(schedule FullConfigSchedule) ([]uint16, uint16) {
	scheduleBlock := make([]uint16, NumSpans*5) // 14 spans * 5 regs
        var spansUsed uint16 = 0;
	for _, span := range schedule.Spans {
		if spansUsed >= NumSpans {
			break
		} // Max 14 spans
                mask := daysToBitmask(span.DaysOfWeek)
                if mask != 0 {
		    offset := int(spansUsed) * 5
		    scheduleBlock[offset+0] = daysToBitmask(span.DaysOfWeek)
		    scheduleBlock[offset+1], scheduleBlock[offset+2] = triggerToPLCData(span.OnTrigger, span.OnTime)
		    scheduleBlock[offset+3], scheduleBlock[offset+4] = triggerToPLCData(span.OffTrigger, span.OffTime)
                    spansUsed++
                }
	}
	return scheduleBlock, spansUsed
}


// PushConfigurationToPLCs orchestrates the full update process.
func PushConfigurationToPLCs(cfg Config, data *FullConfigurationData) error {
	log.Println("Starting Full PLC Configuration Update...")

        applySolarTranslations(cfg, data)

	// Step 1: Push Schedule Times (Start/End/Spans)
	if err := ConfigureSchedules(cfg, data); err != nil {
		log.Printf("Error configuring schedules: %v", err)
		return err
	}


	// Step 2: Global Sync (Force PLC to apply new config)
	if err := GlobalSync(cfg); err != nil {
		log.Printf("Error during Global Sync: %v", err)
		return err
	}

	log.Println("PLC Configuration Update Complete.")
	return nil
}

// ---  ConfigureSchedules ---
func ConfigureSchedules(cfg Config, data *FullConfigurationData) error {
	log.Println("Starting configuration push to all PLCs...")

        // 0. --- Identify Used Schedules ---
	// We only want to upload schedules that are actually assigned to a zone.
	usedScheduleIDs := make(map[int]bool)
	for _, zone := range data.Zones {
		if zone.ScheduleID > 0 {
			usedScheduleIDs[zone.ScheduleID] = true
		}
	}
	log.Printf("Found %d active schedules used by zones.", len(usedScheduleIDs))

	// 1. --- Schedule Remapping ---
	// Create a map of [WordPress_DB_ID] -> [sched_1_to_12]
	dbID_to_schedID := make(map[int]int)

	// Create a map of [sched_1_to_12] -> [70-register-data-block]
	plcScheduleBlocks := make(map[int][]uint16)

        //Prepare the Metadata Block (12 registers for DS1031-DS1042)
	// Initialize with 0s. Index 0 = Slot 1, Index 1 = Slot 2...
	scheduleIDMetadata := make([]uint16, NumSchedules)

        var totalGlobalSpans uint16 = 0
        currentPLCSlot := 1  // 1-based index
	for _, schedule := range data.Schedules {
                var count uint16
                // FILTER: If this schedule isn't used by any zone, skip it.
		if !usedScheduleIDs[schedule.ID] {
			log.Printf("Skipping unused Schedule '%s' (DB ID %d)", schedule.ScheduleName, schedule.ID)
			continue
		}
		if currentPLCSlot > NumSchedules {
			log.Printf("Warning: More than 12 schedules in WordPress. Ignoring schedule '%s' (ID %d) and beyond.", schedule.ScheduleName, schedule.ID)
			break
		} 
                block, count := generateScheduleBlock(schedule)
                if count == 0 {
                        log.Printf("Skipping used Schedule '%s' (DB ID %d) because it contains no valid spans.", schedule.ScheduleName, schedule.ID)
			continue
                }
                dbID_to_schedID[schedule.ID] = currentPLCSlot
		plcScheduleBlocks[currentPLCSlot] = block
                scheduleIDMetadata[currentPLCSlot-1] = uint16(schedule.ID)
                log.Printf("Mapping DB Sched ID %d (%s) -> PLC Sched Slot %d", schedule.ID, schedule.ScheduleName, currentPLCSlot)
                totalGlobalSpans += count
		currentPLCSlot++
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
        for id := range cfg.PLCs {
            plcMaps[id] = make([]uint16, NumLights) 
        }

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
		plcSchedID := dbID_to_schedID[schedDB_ID] // This is the new ID (1-12) or 0
                plcID := mapping.PLCID

                log.Printf("MAPPER: Output %s (Index %d) -> Zone %d -> Sched DB ID %d -> PLC %d Slot %d", 
                    mapping.PLCOutputs[0], loopIndex, zoneID, schedDB_ID, plcID, plcSchedID)
		
		if _, ok := plcMaps[mapping.PLCID]; ok {
			plcMaps[mapping.PLCID][loopIndex] = uint16(plcSchedID)
		}
	}

	// 3. --- Write Blocks to PLCs ---
	for plcID, host := range cfg.PLCs {
		log.Printf("Connecting to PLC %d at %s...", plcID, host)
		handler := modbus.NewTCPClientHandler(host)
		handler.Timeout = 10 * time.Second
                handler.SlaveId = byte(plcID)
		client := modbus.NewClient(handler)
		err := handler.Connect()
		if err != nil {
			log.Printf("  - ERROR connecting to PLC %d: %v", plcID, err)
			continue
		}
		defer handler.Close()

                // 0. Write Total Spans (DS99)
                ds99Data := []uint16{totalGlobalSpans}
                _, err = client.WriteMultipleRegisters(uint16(AddrTotalSpans), 1, u16SliceToBytes(ds99Data))
                if err != nil {
                    log.Printf("Error writing Total Spans (DS99) to PLC %s: %v", host, err)
                    // Decide if you want to 'continue' or try to write the rest anyway
                }

		// A. Write all 12 Schedule Blocks
		log.Printf("  - Writing 12 schedule blocks to PLC %d...", plcID)
		for i := 1; i <= NumSchedules; i++ {
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

                // B.  Write Schedule ID Metadata (DS1031-DS1042)
		log.Printf("  - Writing Schedule ID Metadata (DS1031+) to PLC %d...", plcID)
		_, err = client.WriteMultipleRegisters(AddrSchedIDMapBase, uint16(len(scheduleIDMetadata)), u16SliceToBytes(scheduleIDMetadata))
		if err != nil {
			log.Printf("  - ERROR writing metadata: %v", err)
		}

		// C. Write the 24-register Map Block (which sched slot to use for each light)
		mapBlock, ok := plcMaps[plcID]
		if !ok {
			log.Printf("  - ERROR: No map block found for PLC %d", plcID)
			continue
		}
		
		log.Printf("  - Writing 24-register map block to PLC %d...", plcID)
		_, err = client.WriteMultipleRegisters(AddrLightSchedMapBase, uint16(len(mapBlock)), u16SliceToBytes(mapBlock))
		if err != nil {
			log.Printf("  - ERROR writing map block to PLC %d: %v", plcID, err)
		}

	}
	return nil
}



// PulseZone
func PulseZone(cfg Config, configData *FullConfigurationData, zoneID int, state string) error {
	log.Printf("Received override for Zone %d. Finding ALL associated lights...", zoneID)

	// --- Create a list of all lights to pulse ---
	type pulseTarget struct {
                plcID     int
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

				targets = append(targets, pulseTarget{
					plcID: mapping.PLCID, 
					host: host, 
					loopIndex: loopIndex, 
					outputs: mapping.PLCOutputs})

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
		onCbitAddr  := uint16(AddrRequestOnBase  + target.loopIndex)
		offCbitAddr := uint16(AddrRequestOffBase + target.loopIndex)
		var addrToSet uint16
		var stateStr string

		if state == "on" {
			addrToSet = onCbitAddr
			stateStr = fmt.Sprintf("RequestON (C%d)", 201+target.loopIndex)
		} else {
			addrToSet = offCbitAddr
			stateStr = fmt.Sprintf("RequestOFF (C%d)", 251+target.loopIndex)

                        // --- CLEAR TIMERS ON MANUAL OFF ---
                        // If we are turning the zone off, ensure we clear any active QR timers
                        ClearZoneQROff(cfg, configData, zoneID)
		}

		log.Printf("  -> Pulsing %s (%s) on PLC %s (Loop %d)", stateStr, target.outputs[0], target.host, target.loopIndex+1)

		// Send the pulse
		err := setPLCBit(target.plcID, target.host, addrToSet)
		if err != nil {
			log.Printf("  -> ERROR pulsing %s: %v", target.host, err)
			lastErr = err // Store the last error we saw
		}
	}

	return lastErr // Return nil if no errors, or the last error encountered
}



// ReadStatusFromPLCs
func ReadStatusFromPLCs(cfg Config, configData *FullConfigurationData) (map[string]interface{}, error) {
    fullStatus := make(map[string]interface{})

    // 1. Build Lookups
    loopIndexToMapKey := make(map[string]string)

    for _, mapping := range configData.Mappings {
        if len(mapping.PLCOutputs) == 0 { continue }
        loopIndex := calculateLoopIndex(mapping.PLCOutputs[0])
        if loopIndex == -1 { continue }

        lookupID := fmt.Sprintf("%d-%d", mapping.PLCID, loopIndex)
        loopIndexToMapKey[lookupID] = fmt.Sprintf("PLC%d-%s", mapping.PLCID, mapping.PLCOutputs[0])
    }

    for plc, host := range cfg.PLCs {
        handler := modbus.NewTCPClientHandler(host)
        handler.Timeout = 5 * time.Second
        handler.SlaveId = byte(plc)
        client := modbus.NewClient(handler)
        if err := handler.Connect(); err != nil { continue }
        defer handler.Close()

        // 1. Read the Metadata Map  (DS1031-DS1042)
	plcSlotToDBID := make(map[int]int)
	
	resMeta, err := client.ReadHoldingRegisters(AddrSchedIDMapBase, NumSchedules)
	if err == nil {
		metaVals := bytesToU16Slice(resMeta)
		for i, dbID := range metaVals {
			slotID := i + 1
			if dbID > 0 {
				plcSlotToDBID[slotID] = int(dbID)
			}
		}
	} else {
		log.Printf("PLC %d: Failed to read Schedule Metadata: %v", plc, err)
	}

        // 2. Read Light States (C101-C124) ---
        resCoils, err := client.ReadCoils(AddrLightStateBase, NumLights)
        if err == nil {
            //log.Printf("PLC %d Addr %d Raw Coil Data: %02x", plc, AddrLightStateBase, resCoils)
            for i := 0; i < NumLights; i++ {
                if key, ok := loopIndexToMapKey[fmt.Sprintf("%d-%d", plc, i)]; ok {
                    fullStatus[key] = (resCoils[i/8] >> uint(i%8)) & 1 == 1
                }
            }
        } else {
            log.Printf("PLC %d READ ERROR: %v", plc, err)
        }
        

        // 3. Read Schedule Mapping (DS1000-DS1023)
	// Now uses the map we just read from DS1031!
	resMap, _ := client.ReadHoldingRegisters(AddrLightSchedMapBase, NumLights)
	scheduleMap := make([]int, NumLights)
		
	if len(resMap) > 0 {
		mapVals := bytesToU16Slice(resMap)
		for i, val := range mapVals {
			// Translate Slot -> DB ID
			if dbID, ok := plcSlotToDBID[int(val)]; ok {
				scheduleMap[i] = dbID
			} else {
				scheduleMap[i] = 0 // Unknown or Empty
			}
		}
	}
	fullStatus[fmt.Sprintf("schedule_map_%d", plc)] = scheduleMap

        // 4. Read QR Timers (DS941-DS965) ---
        resQR, err := client.ReadHoldingRegisters(AddrQROffTimeBase, NumLights)
        if err == nil {
            // resQR is a []byte. Each register is 2 bytes.
            // We loop through the registers (i), but pull from the byte slice (resQR)
            for i := 0; i < NumLights; i++ {
                // Combine High Byte and Low Byte
                // Index i*2 is the High Byte, i*2+1 is the Low Byte
                rawVal := uint16(resQR[i*2])<<8 | uint16(resQR[i*2+1])

                if rawVal > 0 {
                    //log.Printf("DEBUG: Found QROff %d at Register Index %d on PLC %d", rawVal, i, plc)
        
                    for _, m := range configData.Mappings {
                        // Now 'i' correctly matches the 0-23 offset calculateLoopIndex returns
                        if m.PLCID == plc && calculateLoopIndex(m.PLCOutputs[0]) == i {
                            if len(m.LinkedZoneIDs) > 0 {
                                fullStatus[fmt.Sprintf("qroff_zone_%d", m.LinkedZoneIDs[0])] = rawVal
                            }
                        }
                    }
                }
            }
        }

        // 5. Read Schedule States (DS1051-DS1062) ---
        //  0=Off, 1=On, 2=QR Enabled
        resStatesBytes, _ := client.ReadHoldingRegisters(AddrSchedStateBase, NumSchedules)
	if len(resStatesBytes) > 0 {
		resStates := bytesToU16Slice(resStatesBytes)
		for i, val := range resStates {
			slotID := i + 1
			if dbID, ok := plcSlotToDBID[slotID]; ok {
				fullStatus[fmt.Sprintf("Sched%d", dbID)] = int(val)
			}
		}
	}

        // 6. Read Photocell (C154)
        if plc == 1 {
            resPhoto, err := client.ReadCoils(AddrPhotocell, 1)
            if err == nil && len(resPhoto) > 0 {
                fullStatus["Photocell"] = (resPhoto[0] & 1) == 1
            }
        }
    }
    return fullStatus, nil
}

// SetZoneQROff updates the DS941+ register for a specific zone with a timestamp.
func SetZoneQROff(cfg Config, configData *FullConfigurationData, zoneID int, qroff uint16) error {
    found := false

    for _, mapping := range configData.Mappings {
        for _, linkedZoneID := range mapping.LinkedZoneIDs {
            if linkedZoneID == zoneID {
                found = true
                host := cfg.PLCs[mapping.PLCID]

                for _, outputStr := range mapping.PLCOutputs {
                    // Convert "Y101" etc to 0-23 index
                    loopIndex := calculateLoopIndex(outputStr)
                    if loopIndex == -1 { continue }

                    // DS941 + offset
                    regAddr := uint16(AddrQROffTimeBase + loopIndex)

                    log.Printf("PLC %d | Zone %d | Light %s -> Writing %04d to DS%d",
                        mapping.PLCID, zoneID, outputStr, qroff, regAddr+1)

                    if err := writeSingleRegister(mapping.PLCID, host, regAddr, qroff); err != nil {
                        return err
                    }
                }
            }
        }
    }
    if !found { return fmt.Errorf("zone %d not found in any PLC mapping", zoneID) }
    return nil
}

// --- Helper Functions ---

// writeSingleRegister helper (needed for the above)
func writeSingleRegister(plcID int, host string, address uint16, value uint16) error {
    handler := modbus.NewTCPClientHandler(host)
    handler.Timeout = 5 * time.Second
    handler.SlaveId = byte(plcID)
    client := modbus.NewClient(handler)
    if err := handler.Connect(); err != nil {
        return err
    }
    defer handler.Close()
    _, err := client.WriteSingleRegister(address, value)
    return err
}


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
        // CLICK PLC Modbus Address Calculation:
        // Y001-Y100 (Outputs 1-100) -> Modbus 0-99   
        // Y101-Y177 (Group 1) -> Modbus 8256+
        // Y201-Y277 (Group 2) -> Modbus 8320+
        // Y301-Y377 (Group 2) -> Modbus 8384+
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

// scheduleIDToModbusAddress now takes a schedule ID (1-12)
func scheduleIDToModbusAddress(schedID int) uint16 {
    // schedule 1 -> DS100 (Modbus 99)
    // schedule 2 -> DS170 (Modbus 169)
    // etc.
    
    // Bounds check using our constant
    if schedID <= 0 || schedID > NumSchedules {
        return 0 
    }

    // Stride is NumSpans (14) * 5 registers per span = 70
    stride := uint16(NumSpans * 5)
    
    // Base is AddrSchedConfigBase (99)
    return uint16(AddrSchedConfigBase + (uint16(schedID-1) * stride))
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

func triggerToPLCData(trigger string, t string) (uint16, uint16) {
    var triggerCode, timeCode uint16

    // 0 = time, 1 = photocell, 2 = QR & time, 3 = QR & photocell
    switch strings.ToUpper(trigger) {
    case "TIME", "SUNDOWN", "SUNRISE":
        triggerCode = 0
    case "PHOTOCELL":
        triggerCode = 1
    case "QR_TIME", "QR_SUNDOWN", "QR_SUNRISE":
        triggerCode = 2
    case "QR_PHOTOCELL":
        triggerCode = 3
    default:
        triggerCode = 0
    }

    if t != "" {
        // 1. Remove colons: "07:15:00" -> "071500"
        cleanTime := strings.ReplaceAll(t, ":", "")
        
        // 2. ONLY take the first 4 characters: "071500" -> "0715"
        if len(cleanTime) >= 4 {
            cleanTime = cleanTime[:4]
        }
        
        if val, err := strconv.Atoi(cleanTime); err == nil {
            timeCode = uint16(val)
        }
    } else {
        timeCode = 0 
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

func bytesToU16Slice(b []byte) []uint16 {
    u16s := make([]uint16, len(b)/2)
    for i := range u16s {
        // Combine High Byte and Low Byte
        u16s[i] = uint16(b[i*2])<<8 | uint16(b[i*2+1])
    }
    return u16s
}

func SetPLCTime(plcID int, host string) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
        handler.SlaveId = byte(plcID)
	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("SetPLCTime connect error: %w", err)
	}
	defer handler.Close()

	now := time.Now()

	// Mapping for CLICK PLC (Contiguous Registers):
	// SD29 (Addr 28): New Year
	// SD30 (Addr 29): New Month
	// SD31 (Addr 30): New Day
	// SD32 (Addr 31): New Day of Week (Required!)
	// SD33 (Addr 32): New Hour
	// SD34 (Addr 33): New Minute
	// SD35 (Addr 34): New Second

	data := []uint16{
		uint16(now.Year()),        // SD29
		uint16(now.Month()),       // SD30
		uint16(now.Day()),         // SD31
		uint16(now.Weekday() + 1), // SD32 (Day of Week: 1=Sun, 2=Mon...)
		uint16(now.Hour()),        // SD33
		uint16(now.Minute()),      // SD34
		uint16(now.Second()),      // SD35
	}

	byteData := u16SliceToBytes(data)
	
	// Write to SD29 (Address 28)
	_, err = client.WriteMultipleRegisters(28, uint16(len(data)), byteData)
	if err != nil {
		return fmt.Errorf("failed to write new time registers: %w", err)
	}

	// Trigger Date Update (SC53 at 61492)
	_, err = client.WriteSingleCoil(61492, 0xFF00) 
	if err != nil {
		return fmt.Errorf("failed to set SC53 (Date Update): %w", err)
	}
	
	// Trigger Time Update (SC55 at 61494)
	_, err = client.WriteSingleCoil(61494, 0xFF00)
	if err != nil {
		return fmt.Errorf("failed to set SC55 (Time Update): %w", err)
	}

	log.Printf("Successfully set time on %s to: %v", host, now.Format(time.RFC3339))
	return nil
}


func setPLCBit(plcID int, host string, address uint16) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
        handler.SlaveId = byte(plcID)
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


// GlobalSync triggers the Schedule Sync (C151) on ALL configured PLCs.
// Call this only after all configuration (Schedules and Modes) has been pushed.
func GlobalSync(cfg Config) error {
	log.Println("Triggering Global Sync on all PLCs...")
	for id, plcAddr := range cfg.PLCs {
		// Use our helper to trigger C151
		triggerPLCSync(id, plcAddr)
		log.Printf("   -> Sync requested for PLC %d (%s)", id, plcAddr)
	}
	return nil
}

// Helper: Sends the "Doorbell Ring" (C151) to a specific PLC to force a Sync
func triggerPLCSync(plcID int, host string) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 2 * time.Second
        handler.SlaveId = byte(plcID)
	client := modbus.NewClient(handler)
	if err := handler.Connect(); err != nil {
		log.Printf("Error connecting to PLC %s for sync: %v", host, err)
		return err
	}
	defer handler.Close()

	// C151 -> Address 16534
	// Calculation: 16384 (Base) + 151 (C number) - 1 = 16534
	addr := uint16(AddrSyncReq)
	
	// WriteSingleCoil 0xFF00 = ON
	_, err := client.WriteSingleCoil(addr, 0xFF00)
	if err != nil {
		log.Printf("Error writing Sync Bit C151 to %s: %v", host, err)
		return err
	}
	return nil
}


// PulseMapping triggers a specific mapping (single light) for testing hardware.
func PulseMapping(cfg Config, configData *FullConfigurationData, mappingID int, state string) error {
	log.Printf("Received TEST command for Mapping ID %d...", mappingID)

	var targetMapping *FullConfigMapping
	for _, m := range configData.Mappings {
		if m.ID == mappingID {
			targetMapping = &m
			break
		}
	}

	if targetMapping == nil {
		return fmt.Errorf("mapping ID %d not found", mappingID)
	}

	if len(targetMapping.PLCOutputs) == 0 {
		return fmt.Errorf("mapping ID %d has no outputs defined", mappingID)
	}

	host, ok := cfg.PLCs[targetMapping.PLCID]
	if !ok {
		return fmt.Errorf("invalid PLCID %d", targetMapping.PLCID)
	}

	loopIndex := calculateLoopIndex(targetMapping.PLCOutputs[0])
	if loopIndex == -1 {
		return fmt.Errorf("invalid output %s", targetMapping.PLCOutputs[0])
	}

	// Calculate addresses
	onCbitAddr  := uint16(AddrRequestOnBase  + loopIndex)
	offCbitAddr := uint16(AddrRequestOffBase + loopIndex)
	var addrToSet uint16
	var stateStr string

	if state == "on" {
		addrToSet = onCbitAddr
		stateStr = fmt.Sprintf("RequestON (C%d)", 201+loopIndex)
	} else {
		addrToSet = offCbitAddr
		stateStr = fmt.Sprintf("RequestOFF (C%d)", 251+loopIndex)
	}

	log.Printf("  -> TEST PULSE: %s on PLC %s", stateStr, host)
	return setPLCBit(targetMapping.PLCID, host, addrToSet)
}

// ClearZoneQROff zeros out the DS941+ registers for all lights in a zone.
func ClearZoneQROff(cfg Config, configData *FullConfigurationData, zoneID int) error {
	for _, mapping := range configData.Mappings {
		for _, linkedZoneID := range mapping.LinkedZoneIDs {
			if linkedZoneID == zoneID {
				host := cfg.PLCs[mapping.PLCID]

				for _, outputStr := range mapping.PLCOutputs {
					loopIndex := calculateLoopIndex(outputStr)
					if loopIndex == -1 { continue }

					// DS941 + offset
					regAddr := uint16(AddrQROffTimeBase + loopIndex)

					log.Printf("CLEARING QR TIMER: PLC %d | Zone %d | Light %s (DS%d)",
						mapping.PLCID, zoneID, outputStr, regAddr+1)

					if err := writeSingleRegister(mapping.PLCID, host, regAddr, 0); err != nil {
						log.Printf("Error clearing register: %v", err)
						return err
					}
				}
			}
		}
	}
	return nil
}



func applySolarTranslations(cfg Config, data *FullConfigurationData) {
    now := time.Now()
    // Bakersfield coordinates from your config
    riseUTC, setUTC := sunrise.SunriseSunset(cfg.Latitude, cfg.Longitude, now.Year(), now.Month(), now.Day())
    
    riseLocal := riseUTC.In(time.Local)
    setLocal := setUTC.In(time.Local)

    // HHMM format for the PLC
    riseInt := riseLocal.Hour()*100 + riseLocal.Minute()
    setInt := setLocal.Hour()*100 + setLocal.Minute()

    log.Printf("Solar Calculation for Today: Sunrise=%04d, Sunset=%04d", riseInt, setInt)

    for sIdx := range data.Schedules {
        for pIdx := range data.Schedules[sIdx].Spans {
            span := &data.Schedules[sIdx].Spans[pIdx]
            
            onTrig := strings.ToUpper(span.OnTrigger)
            if onTrig == "SUNDOWN" || onTrig == "QR_SUNDOWN" {
                span.OnTime = fmt.Sprintf("%04d", setInt)
            } else if onTrig == "SUNRISE" || onTrig == "QR_SUNRISE" {
                span.OnTime = fmt.Sprintf("%04d", riseInt)
            }

            offTrig := strings.ToUpper(span.OffTrigger)
            if offTrig == "SUNDOWN" || offTrig == "QR_SUNDOWN" {
                span.OffTime = fmt.Sprintf("%04d", setInt)
            } else if offTrig == "SUNRISE" || offTrig == "QR_SUNRISE" {
                span.OffTime = fmt.Sprintf("%04d", riseInt)
            }
        }
    }
}


