package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/goburrow/modbus"
)

// PushConfigurationToPLCs translates the DB config and writes it to the PLCs.
func PushConfigurationToPLCs(cfg Config, data *ConfigurationData) error {
	log.Println("Starting configuration push to all PLCs...")
	plcScheduleBlocks := make(map[int]map[int][]uint16)

	for zoneID, scheduleID := range data.Assignments {
		zone, ok := data.Zones[zoneID]; if !ok { continue }
		schedule, ok := data.Schedules[scheduleID]; if !ok { continue }
		if len(zone.MappingIDs) == 0 { continue }
		firstMapping, ok := data.Mappings[zone.MappingIDs[0]]; if !ok { continue }
		plcID := firstMapping.PLCID

		if _, ok := plcScheduleBlocks[plcID]; !ok {
			plcScheduleBlocks[plcID] = make(map[int][]uint16)
		}

		scheduleBlock := make([]uint16, 50)
		for i, span := range schedule.Spans {
			if i >= 10 { break }
			offset := i * 5
			scheduleBlock[offset+0] = daysToBitmask(span.DaysOfWeek)
			scheduleBlock[offset+1], scheduleBlock[offset+2] = triggerToPLCData(span.OnTrigger, span.OnTime)
			scheduleBlock[offset+3], scheduleBlock[offset+4] = triggerToPLCData(span.OffTrigger, span.OffTime)
		}
		plcScheduleBlocks[plcID][schedule.ID] = scheduleBlock
	}

	for plcID, scheduleBlocks := range plcScheduleBlocks {
		plcHost, ok := cfg.PLCs[plcID]
		if !ok {
			log.Printf("Warning: Host for PLC ID %d not found in config.", plcID)
			continue
		}
		log.Printf("Connecting to PLC %d at %s to write %d schedule(s)...", plcID, plcHost, len(scheduleBlocks))
		handler := modbus.NewTCPClientHandler(plcHost)
		handler.Timeout = 10 * time.Second
		client := modbus.NewClient(handler)
		
		for scheduleID, blockData := range scheduleBlocks {
			startAddress := scheduleIDToModbusAddress(scheduleID)
			log.Printf("  - Writing schedule ID %d to start address %d", scheduleID, startAddress)
			
			_, err := client.WriteMultipleRegisters(uint16(startAddress), uint16(len(blockData)), u16SliceToBytes(blockData))
			if err != nil {
				log.Printf("  - ERROR writing schedule %d to PLC %d: %v", scheduleID, plcID, err)
			}
		}
	}
	log.Println("Configuration push finished.")
	return nil
}

// PulseZone finds the correct PLC outputs for a zone and pulses them.
func PulseZone(cfg Config, zoneID int, state string) error {
	configData, err := FetchConfiguration(cfg)
	if err != nil { return fmt.Errorf("could not fetch config for override: %w", err) }

	zone, ok := configData.Zones[zoneID]
	if !ok { return fmt.Errorf("zone with ID %d not found", zoneID) }

	for _, mappingID := range zone.MappingIDs {
		mapping, ok := configData.Mappings[mappingID]
		if !ok { continue }

		if len(mapping.PLCOutputs) != 2 { continue }

		outputToPulse := mapping.PLCOutputs[0] // ON pulse
		if state == "off" {
			outputToPulse = mapping.PLCOutputs[1] // OFF pulse
		}

		plcHost, ok := cfg.PLCs[mapping.PLCID]
		if !ok { continue }

		modbusAddr, err := yOutputToModbusAddress(outputToPulse)
		if err != nil {
			log.Printf("Warning: %v", err)
			continue
		}

		log.Printf("Pulsing output %s (address %d) on PLC %d for zone %d", outputToPulse, modbusAddr, mapping.PLCID, zoneID)
		if err := sendPulse(plcHost, modbusAddr, 250*time.Millisecond); err != nil {
			log.Printf("Error pulsing PLC: %v", err)
		}
	}
	return nil
}

// ReadStatusFromPLCs reads all relevant coils and inputs from all configured PLCs.
func ReadStatusFromPLCs(cfg Config) (map[string]interface{}, error) {
	log.Println("Reading real-time status from all PLCs.")
	
	// Final map to hold all statuses from all PLCs.
	fullStatus := make(map[string]interface{})
	
	configData, err := FetchConfiguration(cfg)
	if err != nil { return nil, fmt.Errorf("could not fetch config for status read: %w", err) }

	// Group mappings by PLC
	mappingsByPLC := make(map[int][]Mapping)
	for _, mapping := range configData.Mappings {
		mappingsByPLC[mapping.PLCID] = append(mappingsByPLC[mapping.PLCID], mapping)
	}

	for plcID, host := range cfg.PLCs {
		log.Printf("Polling PLC %d at %s", plcID, host)
		handler := modbus.NewTCPClientHandler(host)
		handler.Timeout = 5 * time.Second
		client := modbus.NewClient(handler)
		
		// Read Y outputs (Coils)
		// This is a simplified read. A more robust version would find the min/max address and read one block.
		for _, mapping := range mappingsByPLC[plcID] {
			for _, outputName := range mapping.PLCOutputs {
				addr, err := yOutputToModbusAddress(outputName)
				if err != nil { continue }
				
				result, err := client.ReadCoils(addr, 1)
				if err != nil {
					log.Printf("Error reading coil %s on PLC %d: %v", outputName, plcID, err)
					continue
				}
				// The result is a byte slice, we just need the first bit.
				fullStatus[outputName] = (result[0] & 1) == 1
			}
		}

		// Read X inputs (Discrete Inputs)
		// We'll assume photocell is X1 on PLC 1 for this example.
		if plcID == 1 {
			photocellAddr := uint16(0) // X1
			result, err := client.ReadDiscreteInputs(photocellAddr, 1)
			if err != nil {
				log.Printf("Error reading photocell (X1) on PLC 1: %v", err)
			} else {
				fullStatus["Photocell"] = (result[0] & 1) == 1
			}
		}
	}

	return fullStatus, nil
}

// --- Helper Functions ---

func sendPulse(host string, address uint16, duration time.Duration) error {
	handler := modbus.NewTCPClientHandler(host)
	handler.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
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
	switch {
	case num >= 1 && num <= 100:   return uint16(num - 1), nil
	case num >= 101 && num <= 177: return uint16(8256 + (num - 101)), nil
	case num >= 201 && num <= 277: return uint16(8320 + (num - 201)), nil
	case num >= 301 && num <= 377: return uint16(8384 + (num - 301)), nil
	}
	return 0, fmt.Errorf("output number %d is out of supported range", num)
}

func scheduleIDToModbusAddress(id int) uint16 {
	return uint16(99 + (id-1)*50)
}

func daysToBitmask(days []string) uint16 {
	var mask uint16 = 0
	dayMap := map[string]uint16{"Sun": 1, "Mon": 2, "Tue": 4, "Wed": 8, "Thu": 16, "Fri": 32, "Sat": 64}
	for _, day := range days { mask |= dayMap[day] }
	return mask
}

func triggerToPLCData(trigger string, t sql.NullString) (uint16, uint16) {
	var triggerCode, timeCode uint16
	if trigger == "SUNDOWN" || trigger == "SUNRISE" { triggerCode = 1 }
	if trigger == "TIME" { triggerCode = 2 }
	if t.Valid && len(t.String) >= 5 {
		timeStr := strings.Replace(t.String, ":", "", -1)
		timeVal, _ := strconv.Atoi(timeStr[:4])
		timeCode = uint16(timeVal)
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

