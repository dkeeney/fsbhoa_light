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

// --- Data Structures (Mirrors WordPress API Response) ---

type FullConfigZone struct {
	ID         int    `json:"id"`
	ZoneName   string `json:"zone_name"`
	ScheduleID int    `json:"schedule_id"`
}

type FullConfigMapping struct {
	ID          int      `json:"id"`
	PLCID       int      `json:"plc_id"`
	PLCOutputs  []string `json:"plc_outputs"`
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
	OnTime     *string  `json:"on_time"` // Use pointer to string
	OffTrigger string   `json:"off_trigger"`
	OffTime    *string  `json:"off_time"` // Use pointer to string
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
		if zone.ScheduleID == 0 {
			continue
		} // Skip zones with no schedule

		schedule, ok := scheduleMap[zone.ScheduleID]
		if !ok {
			continue
		} // Schedule not found

		// Find the PLC for this zone (via its mappings)
		var plcID int = 0
		for _, mapping := range data.Mappings {
			for _, linkedZoneID := range mapping.LinkedZoneIDs {
				if linkedZoneID == zone.ID {
					plcID = mapping.PLCID
					break
				}
			}
			if plcID != 0 {
				break
			}
		}
		if plcID == 0 {
			continue
		} // No mappings found for this zone

		// Ensure the map for this PLC exists
		if _, ok := plcScheduleBlocks[plcID]; !ok {
			plcScheduleBlocks[plcID] = make(map[int][]uint16)
		}

		// Only generate the block if we haven't already for this schedule
		if _, ok := plcScheduleBlocks[plcID][schedule.ID]; !ok {
			
			// CHANGED: Schedule block is 70 registers (14 spans * 5 regs)
			scheduleBlock := make([]uint16, 70) 
			
			for i, span := range schedule.Spans {
				//  Allow up to 14 spans (index 0-13)
				if i >= 14 {
					break
				}
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
		plcHost, ok := cfg.PLCs[plcID]
		if !ok {
			continue
		}
		log.Printf("Connecting to PLC %d at %s", plcID, plcHost)
		log.Printf("Write %d schedule(s)...", len(scheduleBlocks))

		handler := modbus.NewTCPClientHandler(plcHost)
		handler.Timeout = 10 * time.Second
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
                //  After all schedules are written, set the sync request flag
		log.Println("  - All schedules written. Requesting PLC re-sync...")
		
		// C151 Modbus address is 10240 + (151 - 101) = 10280
		syncRequestAddr, _ := cBitToModbusAddress(151) 
		
		// We use WriteSingleCoil with 0xFF00 to SET the bit
		_, err = client.WriteSingleCoil(syncRequestAddr, 0xFF00) 
		if err != nil {
			log.Printf("  - ERROR requesting re-sync (SET C151) on PLC %d: %v", plcID, err)
		}
	}
	log.Println("Configuration push finished.")
	return nil
}



// PulseZone sends a single bit-set command to the PLC to trigger ReqON or ReqOFF
func PulseZone(cfg Config, configData *FullConfigurationData, zoneID int, state string) error {
	log.Printf("Received override for Zone %d. Finding loop index...", zoneID)

	var plcHost string
	var loopIndex = -1

	for _, mapping := range configData.Mappings {
		for _, linkedZoneID := range mapping.LinkedZoneIDs {
			if linkedZoneID == zoneID {
				plcHost = cfg.PLCs[mapping.PLCID] // Found the host!

				// Calculate index from Y-output
				yName := mapping.PLCOutputs[0] // e.g., "Y101"
				yNum, _ := strconv.Atoi(yName[1:]) // e.g., 101

				moduleGroup := (yNum - (yNum % 100)) / 100 // e.g., 1
				outputOnModule := (yNum % 100)           // e.g., 1

				// Calculate the 0-based index for the *pair*
				// Y101/102 (Output 1) -> (1-1)/2 = 0
				// Y103/104 (Output 3) -> (3-1)/2 = 1
				outputPairIndex := (outputOnModule - 1) / 2

				// 8 pairs per module
				loopIndex = (moduleGroup-1)*8 + outputPairIndex
				break
			}
		}
		if plcHost != "" {
			break
		}
	}

	if plcHost == "" {
		return fmt.Errorf("no host found for ZoneID %d", zoneID)
	}
	if loopIndex == -1 {
		return fmt.Errorf("could not calculate loop index for ZoneID %d", zoneID)
	}

	// Calculate the C-bit addresses based on the 0-based loop index
	onCbitAddr, _ := cBitToModbusAddress(201 + loopIndex) // Loop 0 -> C201
	offCbitAddr, _ := cBitToModbusAddress(251 + loopIndex) // Loop 0 -> C251

	var addrToSet uint16
	var stateStr string

	if state == "on" {
		addrToSet = onCbitAddr
		stateStr = fmt.Sprintf("RequestON (C%d)", 201+loopIndex)
	} else {
		addrToSet = offCbitAddr
		stateStr = fmt.Sprintf("RequestOFF (C%d)", 251+loopIndex)
	}

	log.Printf("Sending SET for %s to PLC %s for Zone %d (Loop %d)", stateStr, plcHost, zoneID, loopIndex+1)
	return setPLCBit(plcHost, addrToSet) // setPLCBit just sets the coil
}

// cBitToModbusAddress helper
func cBitToModbusAddress(cBit int) (uint16, error) {
	// On the CLICK PLUS, C bits start at Modbus address 0.
	// C1 is address 0. C2 is address 1, etc.
	if cBit < 1 {
		return 0, fmt.Errorf("c bit %d is out of supported range", cBit)
	}
	return uint16(cBit - 1), nil
}


// ReadStatusFromPLCs reads all relevant coils and inputs.
func ReadStatusFromPLCs(cfg Config, configData *FullConfigurationData) (map[string]interface{}, error) {
	log.Println("Reading real-time status from all PLCs.")
	fullStatus := make(map[string]interface{})

	// Create a map of [loopIndex] -> [uniqueKey]
	// e.g., 0 -> "PLC1-Y101"
	// This maps the PLC's internal state bit (C101 + loopIndex) to the UI's key
	loopIndexToKey := make(map[int]string)
	
	// Create a map of which PLC holds which loop indices
	plcLoopIndices := make(map[int][]int)

	for _, mapping := range configData.Mappings {
		if len(mapping.PLCOutputs) == 0 {
			continue
		}
		
		// Calculate loopIndex from the first Y output
		yName := mapping.PLCOutputs[0] // e.g., "Y101"
		yNum, _ := strconv.Atoi(yName[1:]) // e.g., 101
		moduleGroup := (yNum - (yNum % 100)) / 100 // e.g., 1
		outputOnModule := (yNum % 100)           // e.g., 1
		outputPairIndex := (outputOnModule - 1) / 2
		loopIndex := (moduleGroup-1)*8 + outputPairIndex // 0-based index

		// The "key" for the UI is based on the *first* (ON) output
		uniqueKey := fmt.Sprintf("PLC%d-%s", mapping.PLCID, yName)
		loopIndexToKey[loopIndex] = uniqueKey
		
		// Track that this PLC is responsible for this loop index
		plcLoopIndices[mapping.PLCID] = append(plcLoopIndices[mapping.PLCID], loopIndex)
	}


	for plcID, host := range cfg.PLCs {
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

		// We read 24 bits starting from C101
		stateBitsAddr, _ := cBitToModbusAddress(101)
		numStateBits := uint16(24)

		resultBytes, err := client.ReadCoils(stateBitsAddr, numStateBits)
		if err != nil {
			log.Printf("  - Error reading state bits (C101-C124) on PLC %d: %v", plcID, err)
		} else {
			// Process the result
			for i := 0; i < int(numStateBits); i++ {
				// Check if this PLC is supposed to have this loop index
				isForThisPLC := false
				for _, plcLoop := range plcLoopIndices[plcID] {
					if plcLoop == i {
						isForThisPLC = true
						break
					}
				}
				
				if !isForThisPLC {
					continue // This PLC doesn't own this loop index
				}

				// Find the UI key for this loop index
				uiKey, ok := loopIndexToKey[i]
				if !ok {
					continue // No mapping for this loop index
				}

				// Check the bit
				byteIndex := i / 8
				bitIndex := uint(i % 8)
				if len(resultBytes) > byteIndex {
					bitValue := (resultBytes[byteIndex] >> bitIndex) & 1
					fullStatus[uiKey] = (bitValue == 1)
				}
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


// yOutputToModbusAddress is now only used for simulation mode
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
	// CLICK PLC Modbus Address Mapping for Y outputs (Coils)
	switch {
	case num >= 1 && num <= 100:
		return uint16(num - 1), nil // Y1=0, Y2=1,... Y100=99
	case num >= 101 && num <= 177:
		return uint16(8256 + (num - 101)), nil // Y101=8256, Y102=8257...
	case num >= 201 && num <= 277:
		return uint16(8320 + (num - 201)), nil // Y201=8320, Y202=8321...
	case num >= 301 && num <= 377:
		return uint16(8384 + (num - 301)), nil // Y301=8384, Y302=8385...
	// Add more modules here if needed
	}
	return 0, fmt.Errorf("output number %d is out of supported range", num)
}

func scheduleIDToModbusAddress(id int) uint16 {
	// D registers start at Modbus address 0. D1 is address 0, D2 is 1...
	// We start our schedules at D100, which is Modbus address 99.
	// The "stride" for each schedule is 70 registers (14 spans * 5 regs)
	// Schedule 1 (DB ID 1) -> D100 (addr 99)
	// Schedule 2 (DB ID 2) -> D170 (addr 169)
	if id <= 0 {
		return 0
	} // Invalid ID
	return uint16(99 + (id-1)*70)
}

func daysToBitmask(days []string) uint16 {
	var mask uint16 = 0
	dayMap := map[string]uint16{"Sun": 1, "Mon": 2, "Tue": 4, "Wed": 8, "Thu": 16, "Fri": 32, "Sat": 64}
	for _, day := range days {
		mask |= dayMap[day]
	}
	return mask
}

func triggerToPLCData(trigger string, t *string) (uint16, uint16) {
	var triggerCode, timeCode uint16

	switch trigger {
	case "TIME":
		triggerCode = 0
	case "SUNDOWN", "SUNRISE":
		triggerCode = 1
	default:
		triggerCode = 0 // Default to TIME
	}

	// t is a pointer. Check if it's not nil and points to a valid string.
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
		bytes[i*2] = byte(v >> 8) // High byte
		bytes[i*2+1] = byte(v)   // Low byte
	}
	return bytes
}

// SetPLCTime writes the current server time to the PLC's clock registers.
func SetPLCTime(host string) error {
	// C2-03CPU RTC Registers (SD20-SD26)
	// SD1 starts at Modbus offset 0.
	// SD20 starts at Modbus offset 19.
	const startAddress = 19 // 0-based offset for SD20

	// 1. Connect to the PLC
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("SetPLCTime connect error: %w", err)
	}
	defer handler.Close()

	// 2. Prepare the data block
	now := time.Now()
	
	// PLC Day of Week is 1-7 (Sun=1, Mon=2...)
	// Go time.Weekday() is 0-6 (Sun=0, Mon=1...)
	plcDayOfWeek := uint16(now.Weekday() + 1)

	data := []uint16{
		uint16(now.Year()),     // SD20: Year (e.g., 2025)
		uint16(now.Month()),    // SD21: Month (1-12)
		uint16(now.Day()),      // SD22: Day (1-31)
		plcDayOfWeek,           // SD23: Day of Week (1-7)
		uint16(now.Hour()),     // SD24: Hour (0-23)
		uint16(now.Minute()),   // SD25: Minute (0-59)
		uint16(now.Second()),   // SD26: Second (0-59)
	}

	// 3. Write the 7 registers
	byteData := u16SliceToBytes(data)

	_, err = client.WriteMultipleRegisters(startAddress, uint16(len(data)), byteData)
	if err != nil {
		return fmt.Errorf("failed to write time registers: %w", err)
	}

	log.Printf("Successfully set time on %s to: %v", host, now.Format(time.RFC3339))
	return nil
}

// setPLCBit connects and SETS a single coil to ON (0xFF00)
func setPLCBit(host string, address uint16) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("setPLCBit connect error: %w", err)
	}
	defer handler.Close()

	// WriteSingleCoil 0xFF00 means SET
	_, err = client.WriteSingleCoil(address, 0xFF00)
	if err != nil {
		return fmt.Errorf("failed to write bit: %w", err)
	}
	return nil
}


