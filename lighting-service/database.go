package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/go-sql-driver/mysql" // Import the driver
)

// --- Data Structures ---

type Zone struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	MappingIDs []int  `json:"mapping_ids"`
}

type Mapping struct {
	ID         int      `json:"id"`
	PLCID      int      `json:"plc_id"`
	PLCOutputs []string `json:"plc_outputs"`
}

type Schedule struct {
	ID    int            `json:"id"`
	Name  string         `json:"name"`
	Spans []ScheduleSpan `json:"spans"`
}

type ScheduleSpan struct {
	DaysOfWeek []string       `json:"days_of_week"`
	OnTrigger  string         `json:"on_trigger"`
	OnTime     sql.NullString `json:"on_time"`
	OffTrigger string         `json:"off_trigger"`
	OffTime    sql.NullString `json:"off_time"`
}

type ConfigurationData struct {
	Zones       map[int]Zone
	Mappings    map[int]Mapping
	Schedules   map[int]Schedule
	Assignments map[int]int
}

// FetchConfiguration connects to the DB and pulls all lighting configuration.
func FetchConfiguration(cfg Config) (*ConfigurationData, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	config := &ConfigurationData{
		Zones:       make(map[int]Zone),
		Mappings:    make(map[int]Mapping),
		Schedules:   make(map[int]Schedule),
		Assignments: make(map[int]int),
	}

	if err := fetchZones(db, config); err != nil { return nil, err }
	if err := fetchMappings(db, config); err != nil { return nil, err }
	if err := fetchSchedules(db, config); err != nil { return nil, err }
	if err := fetchAssignments(db, config); err != nil { return nil, err }

	return config, nil
}

// --- Helper Functions ---

func fetchZones(db *sql.DB, config *ConfigurationData) error {
	rows, err := db.Query("SELECT id, zone_name FROM wp_fsbhoa_lighting_zones")
	if err != nil { return fmt.Errorf("failed to fetch zones: %w", err) }
	defer rows.Close()

	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.ID, &z.Name); err != nil { return err }
		config.Zones[z.ID] = z
	}

	mapRows, err := db.Query("SELECT zone_id, output_id FROM wp_fsbhoa_lighting_zone_output_map")
	if err != nil { return fmt.Errorf("failed to fetch zone mappings: %w", err) }
	defer mapRows.Close()

	for mapRows.Next() {
		var zoneID, outputID int
		if err := mapRows.Scan(&zoneID, &outputID); err != nil { return err }
		if zone, ok := config.Zones[zoneID]; ok {
			zone.MappingIDs = append(zone.MappingIDs, outputID)
			config.Zones[zoneID] = zone
		}
	}
	return nil
}

func fetchMappings(db *sql.DB, config *ConfigurationData) error {
	rows, err := db.Query("SELECT id, plc_id, plc_outputs FROM wp_fsbhoa_lighting_plc_outputs")
	if err != nil { return fmt.Errorf("failed to fetch mappings: %w", err) }
	defer rows.Close()

	for rows.Next() {
		var m Mapping
		var plcOutputsJSON string
		if err := rows.Scan(&m.ID, &m.PLCID, &plcOutputsJSON); err != nil { return err }
		if err := json.Unmarshal([]byte(plcOutputsJSON), &m.PLCOutputs); err != nil { return err }
		config.Mappings[m.ID] = m
	}
	return nil
}

func fetchSchedules(db *sql.DB, config *ConfigurationData) error {
	rows, err := db.Query("SELECT id, schedule_name FROM wp_fsbhoa_lighting_schedules")
	if err != nil { return fmt.Errorf("failed to fetch schedules: %w", err) }
	defer rows.Close()

	for rows.Next() {
		var s Schedule
		if err := rows.Scan(&s.ID, &s.Name); err != nil { return err }
		config.Schedules[s.ID] = s
	}

	spanRows, err := db.Query("SELECT schedule_id, days_of_week, on_trigger, on_time, off_trigger, off_time FROM wp_fsbhoa_lighting_schedule_spans")
	if err != nil { return fmt.Errorf("failed to fetch schedule spans: %w", err) }
	defer spanRows.Close()
	
	for spanRows.Next() {
		var s ScheduleSpan
		var scheduleID int
		var daysOfWeekJSON string
		if err := spanRows.Scan(&scheduleID, &daysOfWeekJSON, &s.OnTrigger, &s.OnTime, &s.OffTrigger, &s.OffTime); err != nil { return err }
		if err := json.Unmarshal([]byte(daysOfWeekJSON), &s.DaysOfWeek); err != nil { return err }
		
		if schedule, ok := config.Schedules[scheduleID]; ok {
			schedule.Spans = append(schedule.Spans, s)
			config.Schedules[scheduleID] = schedule
		}
	}
	return nil
}

func fetchAssignments(db *sql.DB, config *ConfigurationData) error {
	rows, err := db.Query("SELECT zone_id, schedule_id FROM wp_fsbhoa_lighting_zone_schedule_map")
	if err != nil { return fmt.Errorf("failed to fetch assignments: %w", err) }
	defer rows.Close()
	
	for rows.Next() {
		var zoneID, scheduleID int
		if err := rows.Scan(&zoneID, &scheduleID); err != nil { return err }
		config.Assignments[zoneID] = scheduleID
	}
	return nil
}

