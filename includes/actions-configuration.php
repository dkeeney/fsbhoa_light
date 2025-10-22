<?php
// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Register REST API endpoints for the configuration page.
 */
function fsbhoa_config_register_rest_routes() {
    // Endpoints for Zones
    register_rest_route( 'fsbhoa-lighting/v1', '/zones', [
        ['methods' => 'GET', 'callback' => 'fsbhoa_lighting_get_zones', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
        ['methods' => 'POST', 'callback' => 'fsbhoa_lighting_create_or_update_zone', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );
    register_rest_route( 'fsbhoa-lighting/v1', '/zones/(?P<id>\d+)', [
        ['methods' => 'DELETE', 'callback' => 'fsbhoa_lighting_delete_zone', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );

    // Endpoints for Mappings
    register_rest_route( 'fsbhoa-lighting/v1', '/mappings', [
        ['methods' => 'GET', 'callback' => 'fsbhoa_lighting_get_mappings', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
        ['methods' => 'POST', 'callback' => 'fsbhoa_lighting_create_or_update_mapping', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );
    register_rest_route( 'fsbhoa-lighting/v1', '/mappings/(?P<id>\d+)', [
        ['methods' => 'DELETE', 'callback' => 'fsbhoa_lighting_delete_mapping', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );

    // Endpoint for saving assignments from the zone list
    register_rest_route( 'fsbhoa-lighting/v1', '/zone-assignments', [
        'methods' => 'POST',
        'callback' => 'fsbhoa_lighting_save_zone_assignments',
        'permission_callback' => function () { return current_user_can( 'manage_options' ); }
    ] );
}
add_action( 'rest_api_init', 'fsbhoa_config_register_rest_routes' );

/**
 * Creates or updates a zone and its associated output mappings.
 */
function fsbhoa_lighting_create_or_update_zone( WP_REST_Request $request ) {
    global $wpdb;
    $zones_table = $wpdb->prefix . 'fsbhoa_lighting_zones';
    $map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_output_map'; // Zone -> Output Map
    
    $params = $request->get_json_params();
    $zone_id = isset( $params['zone_id'] ) ? intval( $params['zone_id'] ) : 0;
    $zone_name = sanitize_text_field( $params['zone_name'] );
    $description = sanitize_textarea_field( $params['description'] );
    $mapping_ids = isset( $params['mapping_ids'] ) ? array_map( 'intval', $params['mapping_ids'] ) : [];

    if ( empty( $zone_name ) ) { return new WP_REST_Response( [ 'message' => 'Zone name is required.' ], 400 ); }

    $data = ['zone_name' => $zone_name, 'description' => $description];
    
    $wpdb->query('START TRANSACTION');
    try {
        if ( $zone_id > 0 ) {
            if(false === $wpdb->update( $zones_table, $data, [ 'id' => $zone_id ] )) throw new Exception($wpdb->last_error);
        } else {
            if(false === $wpdb->insert( $zones_table, $data )) throw new Exception($wpdb->last_error);
            $zone_id = $wpdb->insert_id;
        }

        // Update Zone -> Output Mappings
        if(false === $wpdb->delete( $map_table, [ 'zone_id' => $zone_id ] )) throw new Exception($wpdb->last_error);
        if ( ! empty( $mapping_ids ) ) {
            foreach ( $mapping_ids as $mapping_id ) {
                if(false === $wpdb->insert( $map_table, [ 'zone_id' => $zone_id, 'output_id' => $mapping_id ] )) throw new Exception($wpdb->last_error);
            }
        }
        $wpdb->query('COMMIT');
        return new WP_REST_Response( [ 'message' => 'Zone saved successfully.', 'id' => $zone_id ], 200 );
    } catch ( Exception $e ) {
        $wpdb->query('ROLLBACK');
        return new WP_REST_Response( [ 'message' => 'Database error: ' . $e->getMessage() ], 500 );
    }
}

/**
 * Fetches all zones, including associated mapping IDs AND assigned schedule ID.
 */
function fsbhoa_lighting_get_zones() {
    global $wpdb;
    $zones_table = $wpdb->prefix . 'fsbhoa_lighting_zones';
    $output_map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_output_map';
    $schedule_map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map'; // Zone -> Schedule Map
    
    $zones = $wpdb->get_results( "SELECT * FROM $zones_table ORDER BY zone_name ASC" );
    if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error fetching zones: ' . $wpdb->last_error], 500);

    if (is_array($zones)) {
        foreach ( $zones as $zone ) {
            // Get mapping IDs
            $zone->mapping_ids = $wpdb->get_col( $wpdb->prepare( "SELECT output_id FROM $output_map_table WHERE zone_id = %d", $zone->id ) );
            if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error fetching mappings: ' . $wpdb->last_error], 500);
            
            // Get assigned schedule ID (will be null if none assigned)
            $zone->schedule_id = $wpdb->get_var( $wpdb->prepare( "SELECT schedule_id FROM $schedule_map_table WHERE zone_id = %d", $zone->id ) );
            if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error fetching assignment: ' . $wpdb->last_error], 500);
            $zone->schedule_id = intval($zone->schedule_id); 
        }
    }
    return new WP_REST_Response( $zones, 200 );
}

/**
 * Deletes a zone and its mappings/assignments.
 */
function fsbhoa_lighting_delete_zone( WP_REST_Request $request ) {
    global $wpdb;
    $zones_table = $wpdb->prefix . 'fsbhoa_lighting_zones';
    $output_map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_output_map';
    $schedule_map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map';
    $zone_id = intval( $request['id'] );
    if($zone_id <= 0) return new WP_REST_Response( [ 'message' => 'Invalid zone ID.' ], 400 );
    
    $wpdb->query('START TRANSACTION');
    try {
        if(false === $wpdb->delete( $output_map_table, [ 'zone_id' => $zone_id ] )) throw new Exception($wpdb->last_error); // Delete output map entries
        if(false === $wpdb->delete( $schedule_map_table, [ 'zone_id' => $zone_id ] )) throw new Exception($wpdb->last_error); // Delete schedule map entries
        if(false === $wpdb->delete( $zones_table, [ 'id' => $zone_id ] )) throw new Exception($wpdb->last_error); // Delete the zone itself
        $wpdb->query('COMMIT');
        return new WP_REST_Response( [ 'message' => 'Zone deleted successfully.' ], 200 );
    } catch (Exception $e) {
        $wpdb->query('ROLLBACK');
        return new WP_REST_Response( [ 'message' => 'Database error: ' . $e->getMessage() ], 500 );
    }
}

/**
 * Saves all zone-to-schedule assignments sent from the zone list.
 */
function fsbhoa_lighting_save_zone_assignments( WP_REST_Request $request ) {
    global $wpdb;
    $map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map';
    $assignments = $request->get_json_params(); // Expects an object like { "zoneId1": "scheduleId1", ... }

    if ( ! is_array( $assignments ) && !is_object( $assignments ) ) { // Check if it's an array or object
        return new WP_REST_Response( ['message' => 'Invalid data format.'], 400 );
    }

    $wpdb->query('START TRANSACTION');
    try {
        // Clear all existing assignments (simpler than updating one by one)
        $wpdb->query("TRUNCATE TABLE $map_table");
        if($wpdb->last_error) throw new Exception('Failed to clear assignments table: ' . $wpdb->last_error);

        foreach ( $assignments as $zone_id_str => $schedule_id_str ) {
            $zone_id = intval($zone_id_str);
            $schedule_id = intval($schedule_id_str);

            if ($zone_id > 0 && $schedule_id > 0) { // Only insert if a valid schedule is selected
                 if(false === $wpdb->insert( $map_table, ['zone_id' => $zone_id, 'schedule_id' => $schedule_id] )) {
                     throw new Exception('Failed to insert assignment: ' . $wpdb->last_error);
                 }
            }
        }
        $wpdb->query('COMMIT');
        return new WP_REST_Response( ['message' => 'Assignments saved successfully.'], 200 );
    } catch ( Exception $e ) {
        $wpdb->query('ROLLBACK');
        return new WP_REST_Response(['message' => 'Database error saving assignments: ' . $e->getMessage()], 500);
    }
}


/**
 * Fetches all PLC output mappings.
 */
function fsbhoa_lighting_get_mappings() {
    global $wpdb;
    $table_name = $wpdb->prefix . 'fsbhoa_lighting_plc_outputs';
    $mappings = $wpdb->get_results( "SELECT * FROM $table_name ORDER BY plc_id, id ASC" );
    if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error: ' . $wpdb->last_error], 500);
    return new WP_REST_Response( $mappings, 200 );
}

/**
 * Creates or updates a PLC output mapping.
 */
function fsbhoa_lighting_create_or_update_mapping( WP_REST_Request $request ) {
    global $wpdb;
    $table_name = $wpdb->prefix . 'fsbhoa_lighting_plc_outputs';
    $params = $request->get_json_params();
    $mapping_id = isset( $params['mapping_id'] ) ? intval( $params['mapping_id'] ) : 0;
    
    $plc_outputs_raw = explode(',', $params['plc_outputs']);
    $plc_outputs_sanitized = array_map(function($output) { return sanitize_text_field(trim($output)); }, $plc_outputs_raw);
    $relays_raw = explode(',', $params['relays']);
    $relays_sanitized = array_map(function($relay) { return sanitize_text_field(trim($relay)); }, $relays_raw);

    $data = [
        'plc_id'      => intval( $params['plc_id'] ),
        'description' => sanitize_text_field( $params['description'] ),
        'plc_outputs' => wp_json_encode( $plc_outputs_sanitized ),
        'relays'      => wp_json_encode( $relays_sanitized ),
    ];

    try {
        if ( $mapping_id > 0 ) {
            $result = $wpdb->update( $table_name, $data, [ 'id' => $mapping_id ] );
        } else {
            $result = $wpdb->insert( $table_name, $data );
        }
        if ( $result === false ) throw new Exception($wpdb->last_error);
    } catch ( Exception $e ) {
        return new WP_REST_Response( [ 'message' => 'Database error: ' . $e->getMessage() ], 500 );
    }
    return new WP_REST_Response( [ 'message' => 'Mapping saved successfully.' ], 200 );
}

/**
 * Deletes a PLC output mapping.
 */
function fsbhoa_lighting_delete_mapping( WP_REST_Request $request ) {
    global $wpdb;
    $table_name = $wpdb->prefix . 'fsbhoa_lighting_plc_outputs';
    $mapping_id = intval( $request['id'] );
    if ( $mapping_id <= 0 ) return new WP_REST_Response( [ 'message' => 'Invalid mapping ID.' ], 400 );
    
    try {
        if(false === $wpdb->delete( $table_name, [ 'id' => $mapping_id ] )) throw new Exception($wpdb->last_error);
        return new WP_REST_Response( [ 'message' => 'Mapping deleted successfully.' ], 200 );
    } catch(Exception $e) {
        return new WP_REST_Response( [ 'message' => 'Database error: ' . $e->getMessage() ], 500 );
    }
}
