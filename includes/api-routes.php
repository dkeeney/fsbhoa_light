<?php
// File: fsbhoa_light/includes/api-routes.php

defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Register Custom REST Route for Lighting Service
 */
add_action( 'rest_api_init', function () {
    // This is the endpoint YOUR Go Service calls
    register_rest_route( 'fsbhoa-lighting/v1', '/verify-swipe', array(
        'methods'  => 'GET',
        'callback' => 'fsbhoa_lighting_handle_verify_swipe',
        'permission_callback' => 'fsbhoa_lighting_api_permission',
    ));
});

/**
 * Permission: Validate Go Service API Key
 */
function fsbhoa_lighting_api_permission( $request ) {
    $provided_key = $request->get_header( 'X-API-KEY' );
    $options = get_option( 'fsbhoa_lighting_settings', [] );
    $valid_key = isset($options['go_service_api_key']) ? $options['go_service_api_key'] : '';
    return ( !empty($valid_key) && $provided_key === $valid_key );
}

/**
 * Logic: Call the EXTERNAL Access Control API to verify the email
 */
function fsbhoa_lighting_handle_verify_swipe( $request ) {
    $email = sanitize_email( $request->get_param( 'email' ) );

    if ( ! is_email( $email ) ) {
        return new WP_Error( 'invalid_email', 'Invalid email provided.', array( 'status' => 400 ) );
    }

    // 1. Get Configuration
    $options = get_option( 'fsbhoa_lighting_settings', [] );
    
    // URL: Use configured URL, or fall back to site_url() if missing
    $base_url = !empty($options['access_control_url']) ? $options['access_control_url'] : site_url();
    
    // API Key: Must be manually configured in Lighting Settings
    $ac_api_key = isset($options['access_control_api_key']) ? $options['access_control_api_key'] : '';

    if ( empty($ac_api_key) ) {
        return new WP_Error( 'config_error', 'Lighting Plugin: Access Control API Key is not configured.', array( 'status' => 500 ) );
    }

    // 2. Build the API Request
    $endpoint = '/wp-json/fsbhoa/v1/access/verify-email';
    $api_url = $base_url . $endpoint;
    $api_url = add_query_arg( 'email', $email, $api_url );

    // 3. Make the Request
    $response = wp_remote_get( $api_url, array(
        'headers' => array(
            'X-API-KEY' => $ac_api_key
        ),
        'timeout' => 5,
        'sslverify' => false 
    ));

    if ( is_wp_error( $response ) ) {
        return new WP_Error( 'api_error', 'Failed to connect to Access Control system at ' . $base_url, array( 'status' => 500 ) );
    }

    // 4. Process Response
    $body = wp_remote_retrieve_body( $response );
    $data = json_decode( $body, true );

    if ( isset($data['isValid']) && $data['isValid'] === true ) {
        return new WP_REST_Response( array( 
            'status' => 'approved', 
            'message' => 'Valid swipe confirmed by AC system.' 
        ), 200 );
    } else {
        return new WP_Error( 'access_denied', 'Access Control reported no recent entry.', array( 'status' => 403 ) );
    }
}

