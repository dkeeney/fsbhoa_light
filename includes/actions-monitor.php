<?php
// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Register REST API endpoints for the monitor page.
 */
function fsbhoa_monitor_register_rest_routes() {
    // Endpoint to GET the current status
    register_rest_route( 'fsbhoa-lighting/v1', '/status', array(
        'methods'  => 'GET',
        'callback' => 'fsbhoa_lighting_get_status_from_service',
        'permission_callback' => function () { return current_user_can( 'manage_options' ); }
    ) );

    // Endpoint to POST an override command
    register_rest_route( 'fsbhoa-lighting/v1', '/override', array(
        'methods'  => 'POST',
        'callback' => 'fsbhoa_lighting_send_override_command',
        'permission_callback' => function () { return current_user_can( 'manage_options' ); }
    ) );
}
add_action( 'rest_api_init', 'fsbhoa_monitor_register_rest_routes' );

/**
 * Fetches the real-time status from the Go service.
 * Acts as a secure proxy between the browser and the Go backend.
 */
function fsbhoa_lighting_get_status_from_service() {
    $service_url = 'http://localhost:8085/status'; // Port defined in Go service
    $response = wp_remote_get( $service_url, array('timeout' => 10) );

    if ( is_wp_error( $response ) ) {
        return new WP_REST_Response(
            ['message' => 'Failed to connect to the Go lighting service: ' . $response->get_error_message()],
            503 // Service Unavailable
        );
    }

    $http_code = wp_remote_retrieve_response_code( $response );
    $body = wp_remote_retrieve_body( $response );

    if ($http_code !== 200) {
         return new WP_REST_Response(
            ['message' => 'Go service returned an error: ' . $body],
            $http_code
        );
    }

    $data = json_decode( $body, true );
    if ( json_last_error() !== JSON_ERROR_NONE ) {
        return new WP_REST_Response(['message' => 'Invalid JSON response from Go service.'], 500);
    }

    $data['last_updated'] = time();
    return new WP_REST_Response( $data, 200 );
}

/**
 * Sends an override command to the Go service.
 * @param WP_REST_Request $request The incoming API request.
 * @return WP_REST_Response The API response.
 */
function fsbhoa_lighting_send_override_command( WP_REST_Request $request ) {
    $params = $request->get_json_params();
    $zone_id = isset($params['zone_id']) ? intval($params['zone_id']) : 0;
    $state = isset($params['state']) ? sanitize_key($params['state']) : ''; // 'on' or 'off'

    if ( $zone_id <= 0 || ($state !== 'on' && $state !== 'off') ) {
        return new WP_REST_Response(['message' => 'Invalid parameters provided.'], 400);
    }

    // Construct the URL for the Go service endpoint
    $service_url = sprintf('http://localhost:8085/override/zone/%d/%s', $zone_id, $state);

    $response = wp_remote_post( $service_url, array('timeout' => 10) );

    if ( is_wp_error( $response ) ) {
        return new WP_REST_Response(
            ['message' => 'Failed to send override command to Go service: ' . $response->get_error_message()],
            503
        );
    }

    $http_code = wp_remote_retrieve_response_code( $response );
    $body = wp_remote_retrieve_body( $response );

    if ($http_code !== 200) {
         return new WP_REST_Response(
            ['message' => 'Go service returned an error on override: ' . $body],
            $http_code
        );
    }

    return new WP_REST_Response( ['message' => 'Override command sent successfully.'], 200 );
}
