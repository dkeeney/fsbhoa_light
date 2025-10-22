<?php
// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Register REST API endpoints for the schedules page.
 */
function fsbhoa_schedules_register_rest_routes() {
    // Endpoints for Schedules
    register_rest_route( 'fsbhoa-lighting/v1', '/schedules', [
        ['methods' => 'GET', 'callback' => 'fsbhoa_lighting_get_schedules', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
        ['methods' => 'POST', 'callback' => 'fsbhoa_lighting_create_or_update_schedule', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );
    register_rest_route( 'fsbhoa-lighting/v1', '/schedules/(?P<id>\d+)', [
        ['methods' => 'DELETE', 'callback' => 'fsbhoa_lighting_delete_schedule', 'permission_callback' => function () { return current_user_can( 'manage_options' ); }],
    ] );

}
add_action( 'rest_api_init', 'fsbhoa_schedules_register_rest_routes' );

/**
 * Fetches all schedules, including their time spans.
 */
function fsbhoa_lighting_get_schedules() {
    global $wpdb;
    $schedules_table = $wpdb->prefix . 'fsbhoa_lighting_schedules';
    $spans_table = $wpdb->prefix . 'fsbhoa_lighting_schedule_spans';

    $schedules = $wpdb->get_results("SELECT * FROM $schedules_table ORDER BY schedule_name ASC");
    if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error: ' . $wpdb->last_error], 500);

    if (is_array($schedules)) {
        foreach ($schedules as $schedule) {
            $schedule->spans = $wpdb->get_results($wpdb->prepare("SELECT * FROM $spans_table WHERE schedule_id = %d", $schedule->id));
            if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error: ' . $wpdb->last_error], 500);
            foreach ($schedule->spans as $span) {
                $span->days_of_week = json_decode($span->days_of_week, true);
            }
        }
    }
    return new WP_REST_Response($schedules, 200);
}

/**
 * Creates or updates a schedule and its time spans.
 */
function fsbhoa_lighting_create_or_update_schedule(WP_REST_Request $request) {
    global $wpdb;
    $schedules_table = $wpdb->prefix . 'fsbhoa_lighting_schedules';
    $spans_table = $wpdb->prefix . 'fsbhoa_lighting_schedule_spans';
    
    $params = $request->get_json_params();
    $schedule_id = isset($params['schedule_id']) ? intval($params['schedule_id']) : 0;
    $schedule_name = sanitize_text_field($params['schedule_name']);
    $spans = isset($params['spans']) ? $params['spans'] : [];

    if (empty($schedule_name)) return new WP_REST_Response(['message' => 'Schedule name is required.'], 400);

    $wpdb->query('START TRANSACTION');
    try {
        if ($schedule_id > 0) {
            if(false === $wpdb->update($schedules_table, ['schedule_name' => $schedule_name], ['id' => $schedule_id])) throw new Exception($wpdb->last_error);
        } else {
            if(false === $wpdb->insert($schedules_table, ['schedule_name' => $schedule_name])) throw new Exception($wpdb->last_error);
            $schedule_id = $wpdb->insert_id;
        }

        if(false === $wpdb->delete($spans_table, ['schedule_id' => $schedule_id])) throw new Exception($wpdb->last_error);

        if (!empty($spans)) {
            foreach ($spans as $span) {
                $days = is_array($span['days_of_week']) ? array_map('sanitize_key', $span['days_of_week']) : [];
                $result = $wpdb->insert($spans_table, [
                    'schedule_id'  => $schedule_id,
                    'days_of_week' => wp_json_encode($days),
                    'on_trigger'   => sanitize_text_field($span['on_trigger']),
                    'on_time'      => ($span['on_trigger'] === 'TIME') ? sanitize_text_field($span['on_time']) : null,
                    'off_trigger'  => sanitize_text_field($span['off_trigger']),
                    'off_time'     => ($span['off_trigger'] === 'TIME') ? sanitize_text_field($span['off_time']) : null,
                ]);
                if ($result === false) throw new Exception('Failed to save a time span: ' . $wpdb->last_error);
            }
        }
        $wpdb->query('COMMIT');
        return new WP_REST_Response(['message' => 'Schedule saved successfully.'], 200);
    } catch (Exception $e) {
        $wpdb->query('ROLLBACK');
        return new WP_REST_Response(['message' => 'DB transaction failed: ' . $e->getMessage()], 500);
    }
}

/**
 * Deletes a schedule and its spans.
 */
function fsbhoa_lighting_delete_schedule(WP_REST_Request $request) {
    global $wpdb;
    $schedules_table = $wpdb->prefix . 'fsbhoa_lighting_schedules';
    $spans_table = $wpdb->prefix . 'fsbhoa_lighting_schedule_spans';
    $schedule_id = intval( $request['id'] );

    if ( $schedule_id <= 0 ) return new WP_REST_Response( [ 'message' => 'Invalid schedule ID.' ], 400 );

    $wpdb->query('START TRANSACTION');
    try {
        if(false === $wpdb->delete($spans_table, ['schedule_id' => $schedule_id])) throw new Exception($wpdb->last_error);
        if(false === $wpdb->delete( $schedules_table, [ 'id' => $schedule_id ] )) throw new Exception($wpdb->last_error);
        $wpdb->query('COMMIT');
        return new WP_REST_Response( [ 'message' => 'Schedule deleted successfully.' ], 200 );
    } catch (Exception $e) {
        $wpdb->query('ROLLBACK');
        return new WP_REST_Response( [ 'message' => 'DB error during deletion: ' . $e->getMessage() ], 500 );
    }
}

/**
 * Fetches all zone-to-schedule assignments.
 */
function fsbhoa_lighting_get_assignments() {
    global $wpdb;
    $map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map';
    $results = $wpdb->get_results( "SELECT zone_id, schedule_id FROM $map_table" );
    if ($wpdb->last_error) return new WP_REST_Response(['message' => 'DB error: ' . $wpdb->last_error], 500);

    $assignments = [];
    foreach ( $results as $row ) {
        $assignments[ $row->zone_id ] = $row->schedule_id;
    }
    return new WP_REST_Response( $assignments, 200 );
}

/**
 * Saves the schedule assignment for a single zone.
 */
function fsbhoa_lighting_save_assignments( WP_REST_Request $request ) {
    global $wpdb;
    $map_table = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map';
    $params = $request->get_json_params();

    $zone_id = intval( $params['zone_id'] );
    $schedule_id = intval( $params['schedule_id'] );

    if ( $zone_id <= 0 ) return new WP_REST_Response( ['message' => 'Invalid Zone ID.'], 400 );

    try {
        if(false === $wpdb->delete( $map_table, ['zone_id' => $zone_id] )) throw new Exception($wpdb->last_error);
        if ( $schedule_id > 0 ) {
            if(false === $wpdb->insert( $map_table, ['zone_id' => $zone_id, 'schedule_id' => $schedule_id] )) throw new Exception($wpdb->last_error);
        }
    } catch ( Exception $e ) {
        return new WP_REST_Response(['message' => 'Database error: ' . $e->getMessage()], 500);
    }

    return new WP_REST_Response( ['message' => 'Assignment saved successfully.'], 200 );
}
